package health

type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
)

type DBStatus string

const (
	DBStatusOK      DBStatus = "ok"
	DBStatusDown    DBStatus = "down"
	DBStatusUnknown DBStatus = "unknown"
)

type Health struct {
	status Status
	db     DBStatus
}

func NewHealth(db DBStatus) Health {
	status := StatusOK
	if db != DBStatusOK {
		status = StatusDegraded
	}
	return Health{status: status, db: db}
}

func (h Health) Status() Status { return h.status }

func (h Health) DB() DBStatus { return h.db }

func (h Health) IsHealthy() bool { return h.status == StatusOK }
