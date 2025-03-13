package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"task_queue/utils"
)

type Response struct {
	Message string `json:"message"`
}

func main() {
	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize flow control
	flowControl := NewFlowControl()
	defer flowControl.Shutdown()

	// Create server handler
	myHandler := &MyHandler{
		flowControl: flowControl,
	}

	// Configure server
	server := &http.Server{
		Addr:    ":3000",
		Handler: myHandler,
	}

	// Handle graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		
		log.Println("Shutting down server...")
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server Shutdown error: %v", err)
		}
		cancel()
	}()

	log.Printf("Server starting on http://localhost:3000")
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

type MyHandler struct {
	flowControl *FlowControl
}

func (h *MyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received %s request from %s", r.Method, r.RemoteAddr)
	
	ctx := r.Context()
	job := utils.NewJob(ctx, func(job *utils.Job) error {
		response := Response{Message: "Hello World"}
		w.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(w).Encode(response)
	})

	if err := h.flowControl.CommitJob(job); err != nil {
		log.Printf("Error committing job: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Println("Job committed to queue successfully")
	job.WaitDone()
}

type FlowControl struct {
	jobQueue *utils.JobQueue
	wm       *utils.WorkerManager
}

func NewFlowControl() *FlowControl {
	jobQueue := utils.NewJobQueue(10)
	log.Println("Job queue initialized")

	m := utils.NewWorkerManager(jobQueue)
	// Create worker pool
	for i := 1; i <= 2; i++ {
		if err := m.CreateWorker(i); err != nil {
			log.Printf("Failed to create worker %d: %v", i, err)
		}
	}
	log.Println("Workers initialized")

	control := &FlowControl{
		jobQueue: jobQueue,
		wm:       m,
	}

	log.Println("Flow control initialized")
	return control
}

func (c *FlowControl) CommitJob(job *utils.Job) error {
	return c.jobQueue.PushJob(job)
}

func (c *FlowControl) Shutdown() {
	c.wm.Shutdown()
}
