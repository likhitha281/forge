package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	JobsSubmitted = prometheus.NewCounter(prometheus.CounterOpts{Name: "forge_jobs_submitted_total", Help: "Jobs submitted"})
	JobsCompleted = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "forge_jobs_completed_total", Help: "Jobs completed"}, []string{"result"})
	QueueLeases   = prometheus.NewCounter(prometheus.CounterOpts{Name: "forge_job_leases_total", Help: "Job leases"})
	RPCDuration   = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "forge_rpc_duration_seconds", Help: "RPC latency"}, []string{"method"})
)

func Register() { prometheus.MustRegister(JobsSubmitted, JobsCompleted, QueueLeases, RPCDuration) }
