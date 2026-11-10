package mbvm_odin

import "core:fmt"
import "core:math/bits"
import "core:time"

HDC_WORDS :: 128
Hypervector :: [HDC_WORDS]u64

hamming_distance :: proc "contextless" (a, b: ^Hypervector) -> int {
    dist := 0
    for i in 0..<HDC_WORDS {
        dist += int(bits.count_ones(a[i] ~ b[i]))
    }
    return dist
}

RelationType :: enum u8 {
    RelDependency = 1,
    RelGeneralizes = 2,
    RelComposition = 3,
    RelCall = 4,
}

CSRGraph :: struct {
    row_offsets: []i32,
    col_indices: []i32,
    rel_types:   []RelationType,
    node_count:  int,
    edge_count:  int,
}

ScratchBuffer :: struct {
    visited: []bool,
    queue:   []i32,
    head:    int,
    tail:    int,
}

init_scratch :: proc(node_count: int, allocator := context.allocator) -> ScratchBuffer {
    return ScratchBuffer{
        visited = make([]bool, node_count, allocator),
        queue   = make([]i32, node_count, allocator),
        head    = 0,
        tail    = 0,
    }
}

reset_scratch :: proc(s: ^ScratchBuffer) {
    for i in 0..<len(s.visited) {
        s.visited[i] = false
    }
    s.head = 0
    s.tail = 0
}

traverse_bfs :: proc(g: ^CSRGraph, start_node: i32, scratch: ^ScratchBuffer, max_depth: int = 3) -> []i32 {
    reset_scratch(scratch)
    if start_node < 0 || int(start_node) >= g.node_count {
        return nil
    }

    scratch.visited[start_node] = true
    scratch.queue[scratch.tail] = start_node
    scratch.tail += 1

    current_depth := 0

    for scratch.head < scratch.tail && current_depth < max_depth {
        level_size := scratch.tail - scratch.head
        for _ in 0..<level_size {
            curr := scratch.queue[scratch.head]
            scratch.head += 1

            start_idx := g.row_offsets[curr]
            end_idx   := g.row_offsets[curr + 1]

            for e in start_idx..<end_idx {
                neighbor := g.col_indices[e]
                if !scratch.visited[neighbor] {
                    scratch.visited[neighbor] = true
                    scratch.queue[scratch.tail] = neighbor
                    scratch.tail += 1
                }
            }
        }
        current_depth += 1
    }

    return scratch.queue[:scratch.tail]
}

main :: proc() {
    fmt.println("=== MBVM-DB Odin Native Engine Benchmark ===")

    // 1. HDC 8K Hamming Distance
    h1: Hypervector
    h2: Hypervector
    for i in 0..<HDC_WORDS {
        h1[i] = 0xAAAAAAAAAAAAAAAA
        h2[i] = 0x5555555555555555
    }

    hdc_iters := 1_000_000
    sink := 0
    t0 := time.now()
    for iter in 0..<hdc_iters {
        h1[iter & 127] ~= u64(iter)
        sink += hamming_distance(&h1, &h2)
    }
    hdc_duration := time.since(t0)
    hdc_ns_op := f64(time.duration_nanoseconds(hdc_duration)) / f64(hdc_iters)
    fmt.printfln("HDC 8K Hamming Distance: %.2f ns/op (%v iters, sink=%v)", hdc_ns_op, hdc_iters, sink)

    // 2. Setup CSR Graph (10,000 nodes, 100,000 edges)
    node_count := 10_000
    edges_per_node := 10
    total_edges := node_count * edges_per_node

    row_offsets := make([]i32, node_count + 1)
    col_indices := make([]i32, total_edges)
    rel_types   := make([]RelationType, total_edges)

    edge_idx := 0
    for i in 0..<node_count {
        row_offsets[i] = i32(edge_idx)
        for j in 0..<edges_per_node {
            col_indices[edge_idx] = i32((i + j + 1) % node_count)
            rel_types[edge_idx] = .RelDependency
            edge_idx += 1
        }
    }
    row_offsets[node_count] = i32(edge_idx)

    graph := CSRGraph{
        row_offsets = row_offsets,
        col_indices = col_indices,
        rel_types   = rel_types,
        node_count  = node_count,
        edge_count  = total_edges,
    }

    scratch := init_scratch(node_count)

    // 3. CSR BFS
    bfs_iters := 100_000
    bfs_sink := 0
    t1 := time.now()
    for iter in 0..<bfs_iters {
        start := i32(iter % node_count)
        res := traverse_bfs(&graph, start, &scratch, 3)
        bfs_sink += len(res)
    }
    bfs_duration := time.since(t1)
    bfs_ns_op := f64(time.duration_nanoseconds(bfs_duration)) / f64(bfs_iters)
    fmt.printfln("CSR BFS Traversal (depth 3): %.2f ns/op (%v queries, sink=%v)", bfs_ns_op, bfs_iters, bfs_sink)
}
