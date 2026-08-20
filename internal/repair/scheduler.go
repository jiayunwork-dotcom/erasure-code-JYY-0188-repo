package repair

import (
	"sort"
)

// Job represents a repair job for a single stripe.
type Job struct {
	StripeID string
	Plan     *RepairPlan
}

// Scheduler manages a queue of repair jobs, ordered by priority (highest first).
// In a real system this would integrate with node health and bandwidth limits;
// here it provides the scheduling logic.
type Scheduler struct {
	jobs     []*Job
	maxQueue int
}

// NewScheduler creates a scheduler with the given maximum queue depth. If
// maxQueue is 0, the queue is unbounded.
func NewScheduler(maxQueue int) *Scheduler {
	return &Scheduler{
		maxQueue: maxQueue,
	}
}

// Submit adds a repair job to the scheduler. If the queue is full, it replaces
// the lowest-priority job if the new job has higher priority. Returns true if
// the job was accepted.
func (s *Scheduler) Submit(job *Job) bool {
	if job == nil || job.Plan == nil {
		return false
	}
	if s.maxQueue > 0 && len(s.jobs) >= s.maxQueue {
		// Find the lowest-priority job.
		minIdx := 0
		for i := 1; i < len(s.jobs); i++ {
			if s.jobs[i].Plan.Priority < s.jobs[minIdx].Plan.Priority {
				minIdx = i
			}
		}
		if job.Plan.Priority <= s.jobs[minIdx].Plan.Priority {
			return false
		}
		// Replace the lowest.
		s.jobs[minIdx] = job
		s.sortJobs()
		return true
	}
	s.jobs = append(s.jobs, job)
	s.sortJobs()
	return true
}

// Next removes and returns the highest-priority job, or nil if the queue is
// empty.
func (s *Scheduler) Next() *Job {
	if len(s.jobs) == 0 {
		return nil
	}
	job := s.jobs[0]
	s.jobs = s.jobs[1:]
	return job
}

// Peek returns the highest-priority job without removing it.
func (s *Scheduler) Peek() *Job {
	if len(s.jobs) == 0 {
		return nil
	}
	return s.jobs[0]
}

// Len returns the current queue length.
func (s *Scheduler) Len() int { return len(s.jobs) }

// Clear removes all jobs from the queue.
func (s *Scheduler) Clear() { s.jobs = nil }

// Drain removes and returns all jobs in priority order.
func (s *Scheduler) Drain() []*Job {
	out := s.jobs
	s.jobs = nil
	return out
}

// sortJobs orders the queue by descending priority.
func (s *Scheduler) sortJobs() {
	sort.Slice(s.jobs, func(i, j int) bool {
		return s.jobs[i].Plan.Priority > s.jobs[j].Plan.Priority
	})
}

// BatchPlan evaluates multiple stripes and returns repair jobs for those that
// need repair, sorted by priority (highest first). Stripes that are healthy or
// unrecoverable are excluded.
func BatchPlan(stripes map[string]*Stripe) []*Job {
	var jobs []*Job
	for id, st := range stripes {
		plan, err := Plan(st)
		if err != nil {
			continue
		}
		jobs = append(jobs, &Job{StripeID: id, Plan: plan})
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Plan.Priority > jobs[j].Plan.Priority
	})
	return jobs
}

// EstimateBandwidth returns the total number of shard reads required to execute
// all jobs in the queue. This is a proxy for network bandwidth consumption.
func (s *Scheduler) EstimateBandwidth() int {
	total := 0
	for _, j := range s.jobs {
		total += len(j.Plan.ReadFrom)
	}
	return total
}

// FilterByPriority returns jobs whose priority is at least minPriority.
func (s *Scheduler) FilterByPriority(minPriority int) []*Job {
	var out []*Job
	for _, j := range s.jobs {
		if j.Plan.Priority >= minPriority {
			out = append(out, j)
		}
	}
	return out
}
