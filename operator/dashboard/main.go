package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

type Job struct {
	Metadata struct {
		UID               string `json:"uid"`
		Name              string `json:"name"`
		Namespace         string `json:"namespace"`
		CreationTimestamp string `json:"creationTimestamp"`
	} `json:"metadata"`
	Status struct {
		Active    int `json:"active"`
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	} `json:"status"`
}

type JobListResponse struct {
	Items []Job `json:"items"`
}

type JobDetails struct {
	Job  batchv1.Job  `json:"job"`
	Pods []corev1.Pod `json:"pods"`
}

type JobDetailsView struct {
	Job      batchv1.Job
	Pods     []corev1.Pod
	Start    string
	Finish   string
	Duration string
}

var templates = template.Must(template.New("tmpl").ParseGlob("templates/*.html"))

func main() {
	backend := os.Getenv("BACKEND_URL")
	if backend == "" {
		backend = "http://localhost:8080"
	}

	fs := http.FileServer(http.Dir("static"))

	mux := http.NewServeMux()
	mux.Handle("/", fs)

	mux.HandleFunc("/frontend/jobs", func(w http.ResponseWriter, r *http.Request) {
		namespace := getNamespace(r.FormValue("namespace"))
		url := fmt.Sprintf("%s/jobs?namespace=%s", backend, namespace)
		body, err := callBackend(url)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var parsed JobListResponse
		json.Unmarshal(body, &parsed)

		templates.ExecuteTemplate(w, "job_list.html", parsed.Items)
	})

	mux.HandleFunc("/frontend/job/details", func(w http.ResponseWriter, r *http.Request) {
		namespace := getNamespace(r.FormValue("namespace"))
		name := r.FormValue("name")

		url := fmt.Sprintf("%s/jobs/details?namespace=%s&name=%s", backend, namespace, name)
		body, err := callBackend(url)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var details JobDetails
		json.Unmarshal(body, &details)

		// Extract timestamps
		var startStr, finishStr, durationStr string

		if details.Job.Status.StartTime != nil {
			t := details.Job.Status.StartTime.Time
			startStr = t.Format(time.RFC3339)
		}

		if details.Job.Status.CompletionTime != nil {
			t := details.Job.Status.CompletionTime.Time
			finishStr = t.Format(time.RFC3339)
		}

		if details.Job.Status.StartTime != nil && details.Job.Status.CompletionTime != nil {
			d := details.Job.Status.CompletionTime.Time.Sub(details.Job.Status.StartTime.Time)
			durationStr = d.Round(time.Second).String()
		}

		view := JobDetailsView{
			Job:      details.Job,
			Pods:     details.Pods,
			Start:    startStr,
			Finish:   finishStr,
			Duration: durationStr,
		}

		templates.ExecuteTemplate(w, "job_details.html", view)
	})

	mux.HandleFunc("/frontend/pod/logs", func(w http.ResponseWriter, r *http.Request) {
		namespace := getNamespace(r.FormValue("namespace"))
		pod := r.FormValue("pod")

		backendURL := fmt.Sprintf("%s/pod/logs?pod=%s&namespace=%s", backend, pod, namespace)
		body, err := callBackend(backendURL)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var data struct {
			Logs string `json:"logs"`
		}
		json.Unmarshal(body, &data)
		templates.ExecuteTemplate(w, "pod_logs.html", map[string]string{
			"Namespace": namespace,
			"Pod":       pod,
			"Logs":      data.Logs,
		})
	})

	mux.Handle("/pw/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		parts := strings.SplitN(strings.TrimPrefix(path, "/pw/"), "/", 2)
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}

		uid := parts[0]
		root := filepath.Join("/playwright-results", uid)
		fs := http.StripPrefix("/pw/"+uid+"/", http.FileServer(http.Dir(root)))
		fs.ServeHTTP(w, r)
	}))

	addr := ":3000"
	log.Printf("Dashboard running on %s", addr)
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

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %s", r.Host, r.UserAgent(), r.Method, r.URL.String())
	})
}

func callBackend(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	return body, nil
}

func getNamespace(namespace string) string {
	if namespace == "" {
		namespace = os.Getenv("DEFAULT_NAMESPACE")
	}

	return namespace
}
