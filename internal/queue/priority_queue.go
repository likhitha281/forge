package queue

import "container/heap"

type Item struct {
	ID       string
	Priority int
	Seq      int64
	index    int
}
type PriorityQueue []*Item

func (p PriorityQueue) Len() int { return len(p) }
func (p PriorityQueue) Less(i, j int) bool {
	if p[i].Priority == p[j].Priority {
		return p[i].Seq < p[j].Seq
	}
	return p[i].Priority < p[j].Priority
}
func (p PriorityQueue) Swap(i, j int) { p[i], p[j] = p[j], p[i]; p[i].index = i; p[j].index = j }
func (p *PriorityQueue) Push(x any)   { *p = append(*p, x.(*Item)) }
func (p *PriorityQueue) Pop() any     { old := *p; n := len(old); x := old[n-1]; *p = old[:n-1]; return x }
func Init(p *PriorityQueue)           { heap.Init(p) }
