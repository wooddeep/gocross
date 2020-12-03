package tool

import (
	"container/list"
	"fmt"
	"sync"
)

// Queue define
type Queue struct {
	data *list.List
	lock sync.Mutex
}

// NewQueue: Constructor
func NewQueue() *Queue {
	q := new(Queue)
	q.data = list.New()
	return q
}

func (q *Queue) push(v interface{}) {
	defer q.lock.Unlock()
	q.lock.Lock()
	q.data.PushFront(v)
}

func (q *Queue) pop() interface{} {
	defer q.lock.Unlock()
	q.lock.Lock()
	iter := q.data.Back()
	v := iter.Value
	q.data.Remove(iter)
	return v
}

func (q *Queue) dump() {
	defer q.lock.Unlock()
	q.lock.Lock()
	for iter := q.data.Back(); iter != nil; iter = iter.Prev() {
		fmt.Println("item:", iter.Value)
	}
}
