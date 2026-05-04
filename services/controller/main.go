package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	mu      sync.Mutex
	active  = map[string]context.CancelFunc{} // jobID → cancel for log goroutine
)

func main() {
	ctx := context.Background()

	db, err := pgxpool.New(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}

	k8s, err := newK8sClient()
	if err != nil {
		log.Fatalf("k8s: %v", err)
	}

	ns := envOr("RUNNER_NAMESPACE", "bosun")
	defaultImage := envOr("DEFAULT_RUNNER_IMAGE", "ghcr.io/duchaineo1/bosun-runner:latest")
	pullSecret := os.Getenv("IMAGE_PULL_SECRET")

	log.Printf("controller started  namespace=%s  default_image=%s  pull_secret=%s", ns, defaultImage, pullSecret)

	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		if err := reconcile(ctx, db, k8s, ns, defaultImage, pullSecret); err != nil {
			log.Printf("reconcile: %v", err)
		}
	}
}

func reconcile(ctx context.Context, db *pgxpool.Pool, k8s *kubernetes.Clientset, ns, defaultImage, pullSecret string) error {
	if err := startPending(ctx, db, k8s, ns, defaultImage, pullSecret); err != nil {
		return fmt.Errorf("startPending: %w", err)
	}
	if err := syncRunning(ctx, db, k8s, ns); err != nil {
		return fmt.Errorf("syncRunning: %w", err)
	}
	return nil
}

func startPending(ctx context.Context, db *pgxpool.Pool, k8s *kubernetes.Clientset, ns, defaultImage, pullSecret string) error {
	rows, err := db.Query(ctx,
		`SELECT id, image_used, playbook, extra_vars::text
		 FROM jobs WHERE status='pending' ORDER BY created_at LIMIT 10`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var jobID, imageUsed, playbook, extraVars string
		if err := rows.Scan(&jobID, &imageUsed, &playbook, &extraVars); err != nil {
			continue
		}
		if imageUsed == "" {
			imageUsed = defaultImage
		}
		if err := spawnJob(ctx, db, k8s, ns, jobID, imageUsed, playbook, extraVars, pullSecret); err != nil {
			log.Printf("spawn job %s: %v", jobID, err)
		}
	}
	return nil
}

func syncRunning(ctx context.Context, db *pgxpool.Pool, k8s *kubernetes.Clientset, ns string) error {
	rows, err := db.Query(ctx,
		"SELECT id, k8s_job_name FROM jobs WHERE status='running' AND k8s_job_name IS NOT NULL")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var jobID, k8sName string
		if err := rows.Scan(&jobID, &k8sName); err != nil {
			continue
		}
		checkK8sJob(ctx, db, k8s, ns, jobID, k8sName)
	}
	return nil
}

func spawnJob(ctx context.Context, db *pgxpool.Pool, k8s *kubernetes.Clientset, ns, jobID, imageUsed, playbook, extraVars, pullSecret string) error {
	k8sName := "bosun-" + strings.ReplaceAll(jobID, "-", "")[:16]

	i32 := func(v int32) *int32 { return &v }

	var pullSecrets []corev1.LocalObjectReference
	if pullSecret != "" {
		pullSecrets = []corev1.LocalObjectReference{{Name: pullSecret}}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: ns,
			Labels:    map[string]string{"bosun/job-id": jobID},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: i32(600),
			BackoffLimit:            i32(0),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"bosun/job-id": jobID},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: pullSecrets,
					Containers: []corev1.Container{
						{
							Name:  "runner",
							Image: imageUsed,
							Env: []corev1.EnvVar{
								{Name: "JOB_ID", Value: jobID},
								{Name: "PLAYBOOK", Value: playbook},
								{Name: "EXTRA_VARS", Value: extraVars},
							},
						},
					},
				},
			},
		},
	}

	if _, err := k8s.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create k8s job: %w", err)
	}

	if _, err := db.Exec(ctx,
		"UPDATE jobs SET status='running', k8s_job_name=$2, started_at=NOW() WHERE id=$1",
		jobID, k8sName,
	); err != nil {
		return fmt.Errorf("update db: %w", err)
	}

	logCtx, cancel := context.WithCancel(context.Background())
	mu.Lock()
	active[jobID] = cancel
	mu.Unlock()

	go streamLogs(logCtx, db, k8s, ns, jobID, k8sName)

	log.Printf("spawned job %s → k8s/%s  image=%s", jobID, k8sName, imageUsed)
	return nil
}

func streamLogs(ctx context.Context, db *pgxpool.Pool, k8s *kubernetes.Clientset, ns, jobID, k8sName string) {
	defer func() {
		mu.Lock()
		delete(active, jobID)
		mu.Unlock()
	}()

	// Wait for pod to exist and be past Pending
	var podName string
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		pods, err := k8s.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
			LabelSelector: "bosun/job-id=" + jobID,
		})
		if err == nil && len(pods.Items) > 0 && pods.Items[0].Status.Phase != corev1.PodPending {
			podName = pods.Items[0].Name
			break
		}
		time.Sleep(2 * time.Second)
	}

	req := k8s.CoreV1().Pods(ns).GetLogs(podName, &corev1.PodLogOptions{Follow: true})
	stream, err := req.Stream(ctx)
	if err != nil {
		log.Printf("log stream %s: %v", jobID, err)
		return
	}
	defer stream.Close()

	lineNum := 0
	buf := make([]byte, 4096)
	var pending strings.Builder

	flush := func(line string) {
		lineNum++
		db.Exec(context.Background(),
			"INSERT INTO job_logs (job_id, line_num, content) VALUES ($1,$2,$3)",
			jobID, lineNum, line)
	}

	for {
		n, err := stream.Read(buf)
		if n > 0 {
			pending.Write(buf[:n])
			for {
				s := pending.String()
				idx := strings.Index(s, "\n")
				if idx < 0 {
					break
				}
				flush(s[:idx])
				pending.Reset()
				if idx+1 < len(s) {
					pending.WriteString(s[idx+1:])
				}
			}
		}
		if err == io.EOF || err != nil {
			break
		}
	}
	if pending.Len() > 0 {
		flush(pending.String())
	}
}

func checkK8sJob(ctx context.Context, db *pgxpool.Pool, k8s *kubernetes.Clientset, ns, jobID, k8sName string) {
	j, err := k8s.BatchV1().Jobs(ns).Get(ctx, k8sName, metav1.GetOptions{})
	if err != nil {
		return
	}

	var newStatus string
	switch {
	case j.Status.Succeeded > 0:
		newStatus = "success"
	case j.Status.Failed > 0:
		newStatus = "failed"
	}

	if newStatus != "" {
		db.Exec(ctx, "UPDATE jobs SET status=$2, finished_at=NOW() WHERE id=$1", jobID, newStatus)
		mu.Lock()
		if cancel, ok := active[jobID]; ok {
			cancel()
		}
		mu.Unlock()
		log.Printf("job %s → %s", jobID, newStatus)
	}
}

func newK8sClient() (*kubernetes.Clientset, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kc := envOr("KUBECONFIG", os.Getenv("HOME")+"/.kube/config")
		cfg, err = clientcmd.BuildConfigFromFlags("", kc)
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(cfg)
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
