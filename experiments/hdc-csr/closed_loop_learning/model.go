package closedloop

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
)

// Model is a 12 -> 8 tanh -> 1 sigmoid MLP with 113 learned parameters.
// It learns typed-link labels, not MiniLM embeddings or executable procedures.
type Model struct {
	Input      [hiddenCount][featureCount]float64 `json:"input"`
	HiddenBias [hiddenCount]float64               `json:"hidden_bias"`
	Output     [hiddenCount]float64               `json:"output"`
	OutputBias float64                            `json:"output_bias"`
	Steps      int                                `json:"steps"`
}

func newModel(seed int64) Model {
	rng := rand.New(rand.NewPCG(uint64(seed), 17))
	var m Model
	for h := range hiddenCount {
		for f := range featureCount {
			m.Input[h][f] = (rng.Float64()*2 - 1) * 0.3
		}
		m.Output[h] = (rng.Float64()*2 - 1) * 0.3
	}
	return m
}

func (m Model) forward(x [featureCount]float64) (float64, [hiddenCount]float64) {
	var hidden [hiddenCount]float64
	z := m.OutputBias
	for h := range hiddenCount {
		a := m.HiddenBias[h]
		for f, v := range x {
			a += m.Input[h][f] * v
		}
		hidden[h] = math.Tanh(a)
		z += m.Output[h] * hidden[h]
	}
	return 1 / (1 + math.Exp(-max(-40, min(40, z)))), hidden
}

func (m Model) score(x [featureCount]float64) float64 { p, _ := m.forward(x); return p }

func (m *Model) train(ctx context.Context, rows []example, steps int, seed int64) error {
	var positive, negative []int
	for i, row := range rows {
		if row.y == 1 {
			positive = append(positive, i)
		} else {
			negative = append(negative, i)
		}
	}
	if len(positive) == 0 || len(negative) == 0 {
		return fmt.Errorf("training needs both observed links and nonlinks; positives=%d negatives=%d", len(positive), len(negative))
	}
	rng := rand.New(rand.NewPCG(uint64(seed), 29))
	for step := range steps {
		if step%64 == 0 {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("train model: %w", err)
			}
		}
		pool := negative
		if step%2 == 0 {
			pool = positive
		}
		row := rows[pool[rng.IntN(len(pool))]]
		p, h := m.forward(row.x)
		delta := p - row.y
		for j := range hiddenCount {
			gradient := delta * m.Output[j] * (1 - h[j]*h[j])
			for k, x := range row.x {
				m.Input[j][k] -= 0.03 * (gradient*x + 0.0001*m.Input[j][k])
			}
			m.HiddenBias[j] -= 0.03 * gradient
			m.Output[j] -= 0.03 * (delta*h[j] + 0.0001*m.Output[j])
		}
		m.OutputBias -= 0.03 * delta
		m.Steps++
	}
	return m.validate()
}

func (m Model) validate() error {
	finite := func(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
	if m.Steps < 0 || !finite(m.OutputBias) {
		return fmt.Errorf("invalid model metadata or bias")
	}
	for h := range hiddenCount {
		if !finite(m.HiddenBias[h]) || !finite(m.Output[h]) {
			return fmt.Errorf("nonfinite hidden/output parameter")
		}
		for _, w := range m.Input[h] {
			if !finite(w) {
				return fmt.Errorf("nonfinite input parameter")
			}
		}
	}
	return nil
}
