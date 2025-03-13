package utils

import (
	"container/list"
	"context"
	"fmt"
	"sync"
)

// Job represents a task to be executed
type Job struct {
	DoneChan   chan struct{}
	HandleFunc func(j *Job) error
	ctx        context.Context
}

func NewJob(ctx context.Context, handler func(j *Job) error) *Job {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Job{
		DoneChan:   make(chan struct{}, 1),
		HandleFunc: handler,
		ctx:        ctx,
	}
}

func (job *Job) Done() {
	select {
	case job.DoneChan <- struct{}{}:
		close(job.DoneChan)
	default:
		// Channel already closed
	}
}

func (job *Job) WaitDone() {
	select {
	case <-job.DoneChan:
		return
	case <-job.ctx.Done():
		return
	}
}

func (job *Job) Execute() error {
	fmt.Println("job start to execute")
	if err := job.HandleFunc(job); err != nil {
		fmt.Printf("job execution failed: %v\n", err)
		return err
	}
	fmt.Println("job executed successfully")
	return nil
}

// JobQueue represents a thread-safe job queue
type JobQueue struct {
	mu         sync.RWMutex
	noticeChan chan struct{}
	queue      *list.List
	size       int
	capacity   int
	closed     bool
}

func NewJobQueue(cap int) *JobQueue {
	if cap <= 0 {
		cap = 10 // default capacity
	}
	return &JobQueue{
		capacity:   cap,
		queue:      list.New(),
		noticeChan: make(chan struct{}, cap),
	}
}

func (q *JobQueue) PushJob(job *Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return fmt.Errorf("job queue is closed")
	}

	q.size++
	if q.size > q.capacity {
		q.removeLeastJob()
	}

	q.queue.PushBack(job)
	select {
	case q.noticeChan <- struct{}{}:
	default:
		// Channel buffer is full, which is fine as workers will be notified
	}
	return nil
}

func (q *JobQueue) PopJob() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size == 0 || q.closed {
		return nil
	}

	front := q.queue.Front()
	if front == nil {
		return nil
	}
	
	value := front.Value
	if job, ok := value.(*Job); ok {
		q.size--
		q.queue.Remove(front)
		return job
	}
	return nil
}

func (q *JobQueue) removeLeastJob() {
	if q.queue.Len() != 0 {
		back := q.queue.Back()
		if back == nil {
			return
		}
		value := back.Value
		if abandonJob, ok := value.(*Job); ok {
			abandonJob.Done()
			q.queue.Remove(back)
			q.size--
		}
	}
}

func (q *JobQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.closed {
		q.closed = true
		close(q.noticeChan)
		// Complete all remaining jobs
		for q.queue.Len() > 0 {
			front := q.queue.Front()
			if front == nil {
				break
			}
			value := front.Value
			if job, ok := value.(*Job); ok {
				q.queue.Remove(front)
				job.Done()
			}
		}
		q.size = 0
	}
}

func (q *JobQueue) waitJob() <-chan struct{} {
	return q.noticeChan
}

func (q *JobQueue) Size() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.size
}

// WorkerManager manages a pool of workers
type WorkerManager struct {
	jobQueue  *JobQueue
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	workerNum int
}

func NewWorkerManager(jobQueue *JobQueue) *WorkerManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerManager{
		jobQueue: jobQueue,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (m *WorkerManager) CreateWorker(workerName int) error {
	m.wg.Add(1)
	m.workerNum++

	go func(index int) {
		defer m.wg.Done()
		fmt.Printf("Worker %d started successfully\n", index)

		for {
			select {
			case <-m.ctx.Done():
				fmt.Printf("Worker %d shutting down\n", index)
				return
			case <-m.jobQueue.waitJob():
				if job := m.jobQueue.PopJob(); job != nil {
					fmt.Printf("Worker %d processing job\n", index)
					if err := job.Execute(); err != nil {
						fmt.Printf("Worker %d job execution failed: %v\n", index, err)
					}
					job.Done()
				}
			}
		}
	}(workerName)

	return nil
}

func (m *WorkerManager) Shutdown() {
	m.cancel()
	m.jobQueue.Close()
	m.wg.Wait()
}
