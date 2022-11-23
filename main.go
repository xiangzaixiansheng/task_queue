package main

import (
	"fmt"
	"net/http"
	"task_queue/utils"
)

func main() {
	flowControl := NewFlowControl()
	myHandler := MyHandler{
		flowControl: flowControl,
	}
	http.Handle("/", &myHandler)

	http.ListenAndServe(":3000", nil)
}

type MyHandler struct {
	flowControl *FlowControl
}

func (h *MyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("recieve http request")
	job := &utils.Job{
		DoneChan: make(chan struct{}, 1),
		HandleFunc: func(job *utils.Job) error {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("Hello World"))
			return nil
		},
	}

	h.flowControl.CommitJob(job)
	fmt.Println("commit job to job queue success")
	job.WaitDone()
}

type FlowControl struct {
	jobQueue *utils.JobQueue
	wm       *utils.WorkerManager
}

func NewFlowControl() *FlowControl {
	jobQueue := utils.NewJobQueue(10)
	fmt.Println("init job queue success")

	m := utils.NewWorkerManager(jobQueue)
	//注册两个worker
	m.CreateWorker(1)
	m.CreateWorker(2)
	fmt.Println("init worker success")

	control := &FlowControl{
		jobQueue: jobQueue,
		wm:       m,
	}

	fmt.Println("init flowcontrol success")
	return control
}

func (c *FlowControl) CommitJob(job *utils.Job) {
	c.jobQueue.PushJob(job)
	fmt.Println("commit job success")
}
