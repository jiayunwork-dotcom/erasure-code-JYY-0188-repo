package repair

import (
	"testing"
)

func TestPlanBasic(t *testing.T) {
	s := &Stripe{
		DataShards:   4,
		ParityShards: 2,
		Status: []ShardStatus{
			Present, Present, Missing, Present, Present, Present,
		},
	}
	plan, err := Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Rebuild) != 1 || plan.Rebuild[0] != 2 {
		t.Fatalf("expected rebuild=[2], got %v", plan.Rebuild)
	}
	if len(plan.ReadFrom) != 4 {
		t.Fatalf("expected 4 read shards, got %d", len(plan.ReadFrom))
	}
	if plan.Priority != 1 {
		t.Fatalf("expected priority 1, got %d", plan.Priority)
	}
}

func TestPlanHealthy(t *testing.T) {
	s := &Stripe{
		DataShards:   3,
		ParityShards: 2,
		Status:       []ShardStatus{Present, Present, Present, Present, Present},
	}
	_, err := Plan(s)
	if err != ErrNoRepairNeeded {
		t.Fatalf("expected ErrNoRepairNeeded, got %v", err)
	}
}

func TestPlanUnrecoverable(t *testing.T) {
	s := &Stripe{
		DataShards:   4,
		ParityShards: 2,
		Status: []ShardStatus{
			Missing, Missing, Missing, Present, Present, Present,
		},
	}
	_, err := Plan(s)
	if err != ErrUnrecoverable {
		t.Fatalf("expected ErrUnrecoverable, got %v", err)
	}
}

func TestPlanDegraded(t *testing.T) {
	s := &Stripe{
		DataShards:   4,
		ParityShards: 2,
		Status: []ShardStatus{
			Present, Degraded, Present, Present, Present, Missing,
		},
	}
	plan, err := Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	// Should rebuild both the degraded shard and the missing one.
	if len(plan.Rebuild) != 2 {
		t.Fatalf("expected 2 rebuilds, got %v", plan.Rebuild)
	}
}

func TestSchedulerPriority(t *testing.T) {
	sched := NewScheduler(10)
	j1 := &Job{StripeID: "s1", Plan: &RepairPlan{Priority: 1, ReadFrom: []int{0, 1, 2, 3}}}
	j2 := &Job{StripeID: "s2", Plan: &RepairPlan{Priority: 3, ReadFrom: []int{0, 1, 2, 3}}}
	j3 := &Job{StripeID: "s3", Plan: &RepairPlan{Priority: 2, ReadFrom: []int{0, 1, 2, 3}}}
	sched.Submit(j1)
	sched.Submit(j2)
	sched.Submit(j3)
	if sched.Len() != 3 {
		t.Fatalf("expected 3 jobs, got %d", sched.Len())
	}
	// Next should be highest priority.
	next := sched.Next()
	if next.StripeID != "s2" {
		t.Fatalf("expected s2 (priority 3), got %s", next.StripeID)
	}
}

func TestSchedulerMaxQueue(t *testing.T) {
	sched := NewScheduler(2)
	sched.Submit(&Job{StripeID: "a", Plan: &RepairPlan{Priority: 1, ReadFrom: []int{0}}})
	sched.Submit(&Job{StripeID: "b", Plan: &RepairPlan{Priority: 2, ReadFrom: []int{0}}})
	// Queue full. Submit higher priority should evict lowest.
	ok := sched.Submit(&Job{StripeID: "c", Plan: &RepairPlan{Priority: 5, ReadFrom: []int{0}}})
	if !ok {
		t.Fatal("expected job c to be accepted")
	}
	if sched.Len() != 2 {
		t.Fatalf("expected queue size 2, got %d", sched.Len())
	}
	// Submit lower priority should be rejected.
	ok = sched.Submit(&Job{StripeID: "d", Plan: &RepairPlan{Priority: 0, ReadFrom: []int{0}}})
	if ok {
		t.Fatal("expected job d to be rejected")
	}
}

func TestHealthAssess(t *testing.T) {
	stripes := []*Stripe{
		{DataShards: 3, ParityShards: 2, Status: []ShardStatus{Present, Present, Present, Present, Present}},
		{DataShards: 3, ParityShards: 2, Status: []ShardStatus{Present, Missing, Present, Present, Present}},
		{DataShards: 3, ParityShards: 2, Status: []ShardStatus{Missing, Missing, Missing, Present, Present}},
	}
	report := Assess(stripes)
	if report.Healthy != 1 {
		t.Fatalf("expected 1 healthy, got %d", report.Healthy)
	}
	if report.AtRisk != 1 {
		t.Fatalf("expected 1 at-risk, got %d", report.AtRisk)
	}
	if report.Lost != 1 {
		t.Fatalf("expected 1 lost, got %d", report.Lost)
	}
	if !report.NeedsUrgentRepair() {
		t.Fatal("expected urgent repair needed")
	}
}
