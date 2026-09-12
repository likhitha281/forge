package queue

import (
	"container/heap"
	"testing"
)

func TestPriorityQueueOrdersByPriority(t *testing.T) {
	pq := &PriorityQueue{}

	Init(pq)

	heap.Push(pq, &Item{
		ID:       "low",
		Priority: 3,
		Seq:      1,
	})

	heap.Push(pq, &Item{
		ID:       "high",
		Priority: 1,
		Seq:      2,
	})

	heap.Push(pq, &Item{
		ID:       "medium",
		Priority: 2,
		Seq:      3,
	})

	expected := []string{
		"high",
		"medium",
		"low",
	}

	for _, expectedID := range expected {
		item := heap.Pop(pq).(*Item)

		if item.ID != expectedID {
			t.Fatalf(
				"expected %q, got %q",
				expectedID,
				item.ID,
			)
		}
	}
}

func TestPriorityQueueFIFOForEqualPriority(t *testing.T) {
	pq := &PriorityQueue{}

	Init(pq)

	heap.Push(pq, &Item{
		ID:       "first",
		Priority: 2,
		Seq:      1,
	})

	heap.Push(pq, &Item{
		ID:       "second",
		Priority: 2,
		Seq:      2,
	})

	heap.Push(pq, &Item{
		ID:       "third",
		Priority: 2,
		Seq:      3,
	})

	expected := []string{
		"first",
		"second",
		"third",
	}

	for _, expectedID := range expected {
		item := heap.Pop(pq).(*Item)

		if item.ID != expectedID {
			t.Fatalf(
				"expected %q, got %q",
				expectedID,
				item.ID,
			)
		}
	}
}

func TestPriorityQueueLength(t *testing.T) {
	pq := &PriorityQueue{}

	Init(pq)

	if pq.Len() != 0 {
		t.Fatalf(
			"expected empty queue, got length %d",
			pq.Len(),
		)
	}

	heap.Push(pq, &Item{
		ID:       "job",
		Priority: 1,
		Seq:      1,
	})

	if pq.Len() != 1 {
		t.Fatalf(
			"expected length 1, got %d",
			pq.Len(),
		)
	}

	heap.Pop(pq)

	if pq.Len() != 0 {
		t.Fatalf(
			"expected empty queue after pop, got length %d",
			pq.Len(),
		)
	}
}
