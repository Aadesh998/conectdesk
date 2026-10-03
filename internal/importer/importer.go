package importer

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/i18ncat"
	"github.com/zerodha/logf"
)

// Job represents the status of an import job.
type Job struct {
	Running   bool      `json:"running"`
	Logs      []string  `json:"logs"`
	Total     int       `json:"total"`
	Success   int       `json:"success"`
	Errors    int       `json:"errors"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// Importer manages background import jobs. Jobs belong to a tenant: each
// tenant runs one import per namespace at a time and only sees its own.
type Importer struct {
	lo     *logf.Logger
	i18n   *i18ncat.Catalog
	jobs   map[jobKey]*Job
	mu     sync.RWMutex
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// jobKey identifies a tenant's job in a namespace, e.g. its tag import.
type jobKey struct {
	tenantID  int
	namespace string
}

// Opts contains options for initializing the Importer.
type Opts struct {
	Lo   *logf.Logger
	I18n *i18ncat.Catalog
}

// New creates and returns a new instance of the Importer.
func New(opts Opts) *Importer {
	ctx, cancel := context.WithCancel(context.Background())
	i := &Importer{
		lo:     opts.Lo,
		i18n:   opts.I18n,
		jobs:   make(map[jobKey]*Job),
		ctx:    ctx,
		cancel: cancel,
	}
	i.wg.Add(1)
	go i.cleanUp()
	return i
}

// Submit submits a new import job for execution.
func (i *Importer) Submit(tenantID int, namespace string, fn func() error) error {
	key := jobKey{tenantID, namespace}
	i.mu.Lock()

	if status, exists := i.jobs[key]; exists && status.Running {
		i.mu.Unlock()
		return envelope.NewError(envelope.ConflictError,
			i.i18n.For(tenantID).T("importer.importAlreadyInProgress"), nil)
	}

	status := &Job{
		Running:   true,
		Logs:      []string{},
		StartedAt: time.Now(),
	}
	i.jobs[key] = status
	i.mu.Unlock()

	i.lo.Info("starting import job", "tenant_id", tenantID, "namespace", namespace)

	go func() {
		defer func() {
			// Recover from panics
			if r := recover(); r != nil {
				i.mu.Lock()
				status.Logs = append(status.Logs, fmt.Sprintf("Panic: %v", r))
				i.mu.Unlock()
				i.lo.Error("import job panicked", "tenant_id", tenantID, "namespace", namespace, "panic", r)
			}

			i.mu.Lock()
			status.Running = false
			status.EndedAt = time.Now()
			i.mu.Unlock()

			i.lo.Info("import job completed", "tenant_id", tenantID, "namespace", namespace,
				"total", status.Total, "success", status.Success, "errors", status.Errors)
		}()

		if err := fn(); err != nil {
			i.mu.Lock()
			status.Logs = append(status.Logs, fmt.Sprintf("Error: %v", err))
			i.mu.Unlock()
			i.lo.Error("import job failed", "tenant_id", tenantID, "namespace", namespace, "error", err)
		}
	}()

	return nil
}

// GetStatus returns the status of the tenant's import job.
func (i *Importer) GetStatus(tenantID int, namespace string) (*Job, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	status, exists := i.jobs[jobKey{tenantID, namespace}]
	if !exists {
		return nil, envelope.NewError(envelope.NotFoundError,
			i.i18n.For(tenantID).T("validation.notFoundImport"), nil)
	}

	return status, nil
}

// AddLog appends a log message to the job status.
func (i *Importer) AddLog(tenantID int, namespace, message string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if status, exists := i.jobs[jobKey{tenantID, namespace}]; exists {
		status.Logs = append(status.Logs, message)
	}
}

// UpdateCounts updates the success/error counts and total.
func (i *Importer) UpdateCounts(tenantID int, namespace string, total, success, errors int) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if status, exists := i.jobs[jobKey{tenantID, namespace}]; exists {
		if total > 0 {
			status.Total = total
		}
		if success > 0 {
			status.Success += success
		}
		if errors > 0 {
			status.Errors += errors
		}
	}
}

// Close gracefully shuts down the importer.
func (i *Importer) Close() {
	i.cancel()
	i.wg.Wait()
}

// cleanUp periodically removes old completed jobs.
func (i *Importer) cleanUp() {
	defer i.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-i.ctx.Done():
			return
		case <-ticker.C:
			i.mu.Lock()
			now := time.Now()
			for key, status := range i.jobs {
				if !status.Running && now.Sub(status.EndedAt) > 1*time.Hour {
					delete(i.jobs, key)
					i.lo.Debug("cleaned up old import job", "tenant_id", key.tenantID, "namespace", key.namespace)
				}
			}
			i.mu.Unlock()
		}
	}
}
