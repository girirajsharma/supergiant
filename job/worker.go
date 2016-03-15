package job

import (
	"encoding/json"
	"fmt"
	"guber"
	"supergiant/core/storage"
	"time"
)

const (
	interval = time.Second
)

const (
	JobDeployComponent = iota
)

type Performable interface {
	MaxAttempts() int
	Perform(data string) error
}

type Worker struct {
	db   *storage.Client
	kube *guber.Client
}

func NewWorker(db *storage.Client, kube *guber.Client) *Worker {
	return &Worker{db, kube}
}

func (w *Worker) Work() {
	for {

		// find Job where status is QUEUED
		// perform CAS, changing QUEUED to STARTED
		// perform Job
		//    if error
		//      - store on job record -------- error handling / status updates on actual models should be handled by the job
		//      - if jobRec.Attempts < job.MaxAttempts()
		//          change jobRec.Status to QUEUED
		//      - else
		//          change jobRec.Status to FAILED
		//    increment jobRec.attempts

		jobs, err := w.db.JobStorage.List()
		if err != nil {
			panic(err)
		}

		var performer Performable

		for _, job := range jobs {
			if job.Status == "QUEUED" {

				prevValue, err := json.Marshal(job)
				if err != nil {
					panic(err)
				}
				job.Status = "STARTED"
				newValue, err := json.Marshal(job)
				if err != nil {
					panic(err)
				}

				// TODO this should be moved to JobStorage
				key := fmt.Sprintf("/jobs/%s", job.ID)
				w.db.CompareAndSwap(key, string(prevValue), string(newValue))

				switch job.Type {
				case JobDeployComponent:
					performer = DeployComponent{w.db, w.kube}
				}

				if err = performer.Perform(job.Data); err != nil {
					job.Error = err.Error()

					if job.Attempts < performer.MaxAttempts() {
						// Add back to queue for retry
						job.Status = "QUEUED"
					} else {
						job.Status = "FAILED" // failed jobs will naturally build up in queue (for now)
					}
					job.Attempts++ // don't guess we need to increment on success (yet)
					w.db.JobStorage.Update(job.ID, job)

				} else {
					// Job is successful, delete from Queue
					w.db.JobStorage.Delete(job.ID)
				}

			}
		}

		time.Sleep(interval)
	}
}