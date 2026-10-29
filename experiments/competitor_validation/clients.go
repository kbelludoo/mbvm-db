package competitorvalidation

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

type answer struct {
	Callers         []string                      `json:"callers"`
	Milliseconds    float64                       `json:"milliseconds"`
	RPCMilliseconds float64                       `json:"rpc_ms"`
	ReplyBytes      int                           `json:"reply_payload_bytes"`
	DiscardedRanges int                           `json:"discarded_ranges,omitempty"`
	CacheStats      *impact.ModuleCacheQueryStats `json:"cache_stats,omitempty"`
	Error           string                        `json:"error,omitempty"`
}

func ms(start time.Time) float64 { return float64(time.Since(start)) / float64(time.Millisecond) }

func (c *lspClient) initialize(root string) error {
	params := struct {
		ProcessID    int    `json:"processId"`
		RootURI      string `json:"rootUri"`
		Capabilities struct {
			General struct {
				PositionEncodings []string `json:"positionEncodings"`
			} `json:"general"`
		} `json:"capabilities"`
	}{ProcessID: os.Getpid(), RootURI: fileURI(root)}
	params.Capabilities.General.PositionEncodings = []string{"utf-16"}
	b, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("initialize params: %w", err)
	}
	raw, err := c.call("initialize", b)
	if err != nil {
		return err
	}
	var result struct {
		Capabilities struct {
			CallHierarchyProvider bool   `json:"callHierarchyProvider"`
			PositionEncoding      string `json:"positionEncoding"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("initialize result: %w", err)
	}
	if !result.Capabilities.CallHierarchyProvider {
		return fmt.Errorf("gopls does not advertise call hierarchy")
	}
	if e := result.Capabilities.PositionEncoding; e != "" && e != "utf-16" {
		return fmt.Errorf("unsupported negotiated encoding %q", e)
	}
	return c.notify("initialized", json.RawMessage("{}"))
}

func (c *lspClient) prepare(t *target) error {
	b, err := json.Marshal(locationParams{document{t.URI}, t.Position})
	if err != nil {
		return fmt.Errorf("prepare params: %w", err)
	}
	raw, err := c.call("textDocument/prepareCallHierarchy", b)
	if err != nil {
		return err
	}
	var items []hierarchyItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("decode hierarchy items: %w", err)
	}
	if len(items) != 1 {
		return fmt.Errorf("expected one hierarchy item for %s, got %d", t.Name, len(items))
	}
	t.Item = items[0]
	return nil
}

func (c *lspClient) query(t target, cat *catalogue) answer {
	start := time.Now()
	params := struct {
		Item hierarchyItem `json:"item"`
	}{t.Item}
	b, err := json.Marshal(params)
	if err != nil {
		return answer{Error: err.Error(), Milliseconds: ms(start)}
	}
	raw, err := c.call("callHierarchy/incomingCalls", b)
	a := answer{RPCMilliseconds: ms(start), ReplyBytes: len(raw)}
	if err == nil {
		var calls []incomingCall
		err = json.Unmarshal(raw, &calls)
		if err == nil {
			a.Callers, a.DiscardedRanges, err = cat.normalize(calls, t.Name)
		}
	}
	a.Milliseconds = ms(start)
	if err != nil {
		a.Error = err.Error()
	}
	return a
}

func (p *process) mbvmQuery(name string) answer {
	start := time.Now()
	b, err := json.Marshal(struct {
		Symbol string `json:"symbol"`
	}{name})
	if err == nil {
		_, err = p.in.Write(append(b, '\n'))
	}
	p.trace = append(p.trace, wireMessage{"send", b})
	if err != nil {
		return answer{Error: err.Error(), Milliseconds: ms(start)}
	}
	raw, err := p.out.ReadBytes('\n')
	a := answer{RPCMilliseconds: ms(start), ReplyBytes: len(raw)}
	if err == nil {
		p.trace = append(p.trace, wireMessage{"receive", raw})
		var r struct {
			Impact *impact.ModuleImpact          `json:"impact"`
			Stats  *impact.ModuleCacheQueryStats `json:"stats"`
			Error  string                        `json:"error"`
		}
		err = json.Unmarshal(raw, &r)
		if err == nil {
			a.CacheStats = r.Stats
			if r.Error != "" {
				err = fmt.Errorf("MBVM server: %s", r.Error)
			} else if r.Impact == nil {
				err = fmt.Errorf("MBVM missing impact response")
			} else {
				a.Callers = r.Impact.DirectCallers
			}
		}
	}
	a.Milliseconds = ms(start)
	if err != nil {
		a.Error = err.Error()
	}
	return a
}
