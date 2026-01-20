package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Response-Typen für JSON-API

type JobListResponse struct {
	Items    []batchv1.Job `json:"items"`
	Continue string        `json:"continue,omitempty"`
}

type JobDetailsResponse struct {
	Job  *batchv1.Job `json:"job"`
	Pods []corev1.Pod `json:"pods"`
}

func main() {
	clientset, err := newKubeClient()
	if err != nil {
		log.Fatalf("cannot create Kubernetes client: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// GET /jobs?namespace=ns&limit=50&continue=token
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		namespace := getNamespace(r.URL.Query().Get("namespace"))
		listJobs(w, r, clientset, namespace)
	})

	// GET /jobs/details?namespace=ns&name=jobname
	mux.HandleFunc("/jobs/details", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		namespace := getNamespace(r.URL.Query().Get("namespace"))
		name := r.URL.Query().Get("name")
		if namespace == "" || name == "" {
			http.Error(w, "namespace and name parameters required", http.StatusBadRequest)
			return
		}
		jobDetails(w, r, clientset, namespace, name)
	})

	mux.HandleFunc("/pod/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		podLogs(w, r, clientset)
	})

	addr := ":8080"
	log.Printf("REST API listening on %s", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server Error: %v", err)
	}
}

func getNamespace(namespace string) string {
	if namespace == "" {
		namespace = os.Getenv("DEFAULT_NAMESPACE")
	}

	return namespace
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", r.Host, r.UserAgent(), r.Method, r.URL.String())
	})
}

func newKubeClient() (*kubernetes.Clientset, error) {
	in, err := rest.InClusterConfig()
	if err == nil {
		return kubernetes.NewForConfig(in)
	}

	return nil, err
}

func listJobs(w http.ResponseWriter, r *http.Request, clientset *kubernetes.Clientset, namespace string) {
	ctx := context.Background()
	opts := metav1.ListOptions{}

	jobs, err := clientset.BatchV1().Jobs(namespace).List(ctx, opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sort.Slice(jobs.Items, func(i, j int) bool {
		return jobs.Items[i].CreationTimestamp.After(jobs.Items[j].CreationTimestamp.Time)
	})

	resp := JobListResponse{
		Items:    jobs.Items,
		Continue: jobs.Continue,
	}

	respondJSON(w, resp)
}

// /jobs/details Handler
func jobDetails(w http.ResponseWriter, r *http.Request, clientset *kubernetes.Clientset, namespace, name string) {
	ctx := context.Background()

	job, err := clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", name),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := JobDetailsResponse{
		Job:  job,
		Pods: pods.Items,
	}

	respondJSON(w, response)
}

// GET /jobs/logs?namespace=X&pod=Y
func podLogs(w http.ResponseWriter, r *http.Request, clientset *kubernetes.Clientset) {
	namespace := getNamespace(r.URL.Query().Get("namespace"))
	pod := r.URL.Query().Get("pod")

	if namespace == "" || pod == "" {
		http.Error(w, "namespace and pod are required", http.StatusBadRequest)
		return
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{})
	stream, err := req.Stream(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer stream.Close()

	logData, err := io.ReadAll(stream)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	respondJSON(w, map[string]string{
		"logs": string(logData),
	})
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
