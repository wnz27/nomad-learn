package nodedrain

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"

	"github.com/hashicorp/nomad/e2e/e2eutil"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/testutil"
)

const ns = ""

// TestNodeDrainEphemeralMigrate tests that ephermeral_disk migrations work as
// expected even during a node drain.
func TestNodeDrainEphemeralMigrate(t *testing.T) {

	jobIDs := []string{}
	nodeIDs := []string{}
	t.Cleanup(func() { test.NoError(t, cleanupNodeDrainTest(jobIDs, nodeIDs)) })

	jobID := "test-node-drain-" + uuid.Generate()[0:8]
	must.NoError(t, e2eutil.Register(jobID, "./input/drain_migrate.nomad"))
	jobIDs = append(jobIDs, jobID)

	expected := []string{"running"}
	must.NoError(t, e2eutil.WaitForAllocStatusExpected(jobID, ns, expected),
		must.Sprint("job should be running"))

	allocs, err := e2eutil.AllocsForJob(jobID, ns)
	must.NoError(t, err, must.Sprint("could not get allocs for job"))
	must.Len(t, 1, allocs, must.Sprint("could not get allocs for job"))
	oldAllocID := allocs[0]["ID"]

	nodes, err := nodesForJob(jobID)
	must.NoError(t, err, must.Sprint("could not get nodes for job"))
	must.Len(t, 1, nodes, must.Sprint("could not get nodes for job"))
	nodeID := nodes[0]

	t.Logf("draining node %v", nodeID)
	out, err := e2eutil.Command("nomad", "node", "drain", "-enable", "-yes", "-detach", nodeID)
	must.NoError(t, err, must.Sprintf("'nomad node drain' failed: %v\n%v", err, out))
	nodeIDs = append(nodeIDs, nodeID)

	must.NoError(t, waitForNodeDrain(nodeID,
		func(got []map[string]string) bool {
			for _, alloc := range got {
				if alloc["ID"] == oldAllocID && alloc["Status"] == "complete" {
					return true
				}
			}
			return false
		}, &e2eutil.WaitConfig{Interval: time.Millisecond * 100, Retries: 500},
	), must.Sprint("node did not drain"))

	// wait for the allocation to be migrated
	expected = []string{"running", "complete"}
	must.NoError(t, e2eutil.WaitForAllocStatusExpected(jobID, ns, expected),
		must.Sprint("job should be running"))

	allocs, err = e2eutil.AllocsForJob(jobID, ns)
	must.NoError(t, err, must.Sprint("could not get allocations for job"))

	// the task writes its alloc ID to a file if it hasn't been previously
	// written, so find the contents of the migrated file and make sure they
	// match the old allocation, not the running one
	var got string
	var fsErr error
	testutil.WaitForResultRetries(10, func() (bool, error) {
		time.Sleep(time.Millisecond * 100)
		for _, alloc := range allocs {
			if alloc["Status"] == "running" && alloc["Node ID"] != nodeID && alloc["ID"] != oldAllocID {
				got, fsErr = e2eutil.Command("nomad", "alloc", "fs",
					alloc["ID"], fmt.Sprintf("alloc/data/%s", jobID))
				if err != nil {
					return false, err
				}
				return true, nil
			}
		}
		return false, fmt.Errorf("missing expected allocation")
	}, func(e error) {
		fsErr = e
	})
	must.NoError(t, fsErr, must.Sprint("could not get allocation data"))
	must.Eq(t, oldAllocID, strings.TrimSpace(got), must.Sprint("node drained but migration failed"))
}

// TestNodeDrainIgnoreSystem tests that system jobs are left behind when the
// -ignore-system flag is used.
func TestNodeDrainIgnoreSystem(t *testing.T) {

	jobIDs := []string{}
	nodeIDs := []string{}
	t.Cleanup(func() { test.NoError(t, cleanupNodeDrainTest(jobIDs, nodeIDs)) })

	nodes, err := e2eutil.NodeStatusListFiltered(
		func(section string) bool {
			dc, err := e2eutil.GetField(section, "DC")
			must.NoError(t, err)
			kernelName, err := e2eutil.GetField(section, "kernel.name")
			return err == nil && kernelName == "linux" && (dc == "dc1" || dc == "dc2")
		})
	must.NoError(t, err, must.Sprint("could not get node status listing"))

	serviceJobID := "test-node-drain-service-" + uuid.Generate()[0:8]
	systemJobID := "test-node-drain-system-" + uuid.Generate()[0:8]

	must.NoError(t, e2eutil.Register(serviceJobID, "./input/drain_simple.nomad"))
	jobIDs = append(jobIDs, serviceJobID)

	must.NoError(t, e2eutil.WaitForAllocStatusExpected(serviceJobID, ns, []string{"running"}))

	allocs, err := e2eutil.AllocsForJob(serviceJobID, ns)
	must.NoError(t, err, must.Sprint("could not get allocs for service job"))
	must.Len(t, 1, allocs, must.Sprint("could not get allocs for service job"))
	oldAllocID := allocs[0]["ID"]

	must.NoError(t, e2eutil.Register(systemJobID, "./input/drain_ignore_system.nomad"))
	jobIDs = append(jobIDs, systemJobID)

	expected := []string{"running"}
	must.NoError(t, e2eutil.WaitForAllocStatusExpected(serviceJobID, ns, expected),
		must.Sprint("service job should be running"))

	// can't just give it a static list because the number of nodes can vary
	must.NoError(t, e2eutil.WaitForAllocStatusComparison(
		func() ([]string, error) { return e2eutil.AllocStatuses(systemJobID, ns) },
		func(got []string) bool {
			if len(got) != len(nodes) {
				return false
			}
			for _, status := range got {
				if status != "running" {
					return false
				}
			}
			return true
		}, nil,
	),
		must.Sprint("system job should be running on every node"),
	)

	jobNodes, err := nodesForJob(serviceJobID)
	must.NoError(t, err, must.Sprint("could not get nodes for job"))
	must.Len(t, 1, jobNodes, must.Sprint("could not get nodes for job"))
	nodeID := jobNodes[0]

	t.Logf("draining node %v", nodeID)
	out, err := e2eutil.Command(
		"nomad", "node", "drain",
		"-ignore-system", "-enable", "-yes", "-detach", nodeID)
	must.NoError(t, err, must.Sprintf("'nomad node drain' failed: %v\n%v", err, out))
	nodeIDs = append(nodeIDs, nodeID)

	must.NoError(t, waitForNodeDrain(nodeID,
		func(got []map[string]string) bool {
			for _, alloc := range got {
				if alloc["ID"] == oldAllocID && alloc["Status"] == "complete" {
					return true
				}
			}
			return false
		}, &e2eutil.WaitConfig{Interval: time.Millisecond * 100, Retries: 500},
	), must.Sprint("node did not drain"))

	allocs, err = e2eutil.AllocsForJob(systemJobID, ns)
	must.NoError(t, err, must.Sprint("could not query allocs for system job"))
	must.Eq(t, len(nodes), len(allocs), must.Sprint("system job should still be running on every node"))
	for _, alloc := range allocs {
		must.Eq(t, "run", alloc["Desired"], must.Sprint("no system allocs should be draining"))
		must.Eq(t, "running", alloc["Status"], must.Sprint("no system allocs should be draining"))
	}
}

// TestNodeDrainDeadline tests the enforcement of the node drain deadline so
// that allocations are terminated even if they haven't gracefully exited.
func TestNodeDrainDeadline(t *testing.T) {

	jobIDs := []string{}
	nodeIDs := []string{}
	t.Cleanup(func() { test.NoError(t, cleanupNodeDrainTest(jobIDs, nodeIDs)) })

	jobID := "test-node-drain-" + uuid.Generate()[0:8]
	must.NoError(t, e2eutil.Register(jobID, "./input/drain_deadline.nomad"))
	jobIDs = append(jobIDs, jobID)

	expected := []string{"running"}
	must.NoError(t, e2eutil.WaitForAllocStatusExpected(jobID, ns, expected), must.Sprint("job should be running"))

	nodes, err := nodesForJob(jobID)
	must.NoError(t, err, must.Sprint("could not get nodes for job"))
	must.Len(t, 1, nodes, must.Sprint("could not get nodes for job"))
	nodeID := nodes[0]

	t.Logf("draining node %v", nodeID)
	out, err := e2eutil.Command(
		"nomad", "node", "drain",
		"-deadline", "5s",
		"-enable", "-yes", "-detach", nodeID)
	must.NoError(t, err, must.Sprintf("'nomad node drain %v' failed: %v\n%v", nodeID, err, out))
	nodeIDs = append(nodeIDs, nodeID)

	// the deadline is 40s but we can't guarantee its instantly terminated at
	// that point, so we give it 30s which is well under the 2m kill_timeout in
	// the job.
	// deadline here needs to account for scheduling and propagation delays.
	must.NoError(t, waitForNodeDrain(nodeID,
		func(got []map[string]string) bool {
			// FIXME: check the drain job alloc specifically. test
			// may pass if client had another completed alloc
			for _, alloc := range got {
				if alloc["Status"] == "complete" {
					return true
				}
			}
			return false
		}, &e2eutil.WaitConfig{Interval: time.Second, Retries: 40},
	), must.Sprint("node did not drain immediately following deadline"))
}

// TestNodeDrainForce tests the enforcement of the node drain -force flag so
// that allocations are terminated immediately.
func TestNodeDrainForce(t *testing.T) {

	jobIDs := []string{}
	nodeIDs := []string{}
	t.Cleanup(func() { test.NoError(t, cleanupNodeDrainTest(jobIDs, nodeIDs)) })

	jobID := "test-node-drain-" + uuid.Generate()[0:8]
	must.NoError(t, e2eutil.Register(jobID, "./input/drain_deadline.nomad"))
	jobIDs = append(jobIDs, jobID)

	expected := []string{"running"}
	must.NoError(t, e2eutil.WaitForAllocStatusExpected(jobID, ns, expected), must.Sprint("job should be running"))

	nodes, err := nodesForJob(jobID)
	must.NoError(t, err, must.Sprint("could not get nodes for job"))
	must.Len(t, 1, nodes, must.Sprint("could not get nodes for job"))
	nodeID := nodes[0]

	t.Logf("draining node %v", nodeID)
	out, err := e2eutil.Command(
		"nomad", "node", "drain",
		"-force",
		"-enable", "-yes", "-detach", nodeID)
	must.NoError(t, err, must.Sprintf("'nomad node drain' failed: %v\n%v", err, out))
	nodeIDs = append(nodeIDs, nodeID)

	// we've passed -force but we can't guarantee its instantly terminated at
	// that point, so we give it 30s which is under the 2m kill_timeout in
	// the job
	must.NoError(t, waitForNodeDrain(nodeID,
		func(got []map[string]string) bool {
			// FIXME: check the drain job alloc specifically. test
			// may pass if client had another completed alloc
			for _, alloc := range got {
				if alloc["Status"] == "complete" {
					return true
				}
			}
			return false
		}, &e2eutil.WaitConfig{Interval: time.Second, Retries: 40},
	), must.Sprint("node did not drain immediately when forced"))

}

// TestNodeDrainKeepIneligible tests that nodes can be kept ineligible for
// scheduling after disabling drain.
func TestNodeDrainKeepIneligible(t *testing.T) {

	jobIDs := []string{}
	nodeIDs := []string{}
	t.Cleanup(func() { test.NoError(t, cleanupNodeDrainTest(jobIDs, nodeIDs)) })

	nodes, err := e2eutil.NodeStatusList()
	must.NoError(t, err, must.Sprint("could not get node status listing"))

	nodeID := nodes[0]["ID"]

	out, err := e2eutil.Command("nomad", "node", "drain", "-enable", "-yes", "-detach", nodeID)
	must.NoError(t, err, must.Sprintf("'nomad node drain' failed: %v\n%v", err, out))
	nodeIDs = append(nodeIDs, nodeID)

	t.Logf("draining node %v", nodeID)
	_, err = e2eutil.Command(
		"nomad", "node", "drain",
		"-disable", "-keep-ineligible", "-yes", nodeID)
	must.NoError(t, err, must.Sprintf("'nomad node drain' failed: %v\n%v", err, out))

	nodes, err = e2eutil.NodeStatusList()
	must.NoError(t, err, must.Sprint("could not get updated node status listing"))

	must.Eq(t, "ineligible", nodes[0]["Eligibility"])
	must.Eq(t, "false", nodes[0]["Drain"])
}

// ---------------------------------------
// test helpers

func cleanupNodeDrainTest(jobIDs, nodeIDs []string) error {
	for _, jobID := range jobIDs {
		err := e2eutil.StopJob(jobID, "-purge", "-detach")
		if err != nil {
			return err
		}
	}
	for _, id := range nodeIDs {
		_, err := e2eutil.Command("nomad", "node", "drain", "-disable", "-yes", id)
		if err != nil {
			return err
		}
		_, err = e2eutil.Command("nomad", "node", "eligibility", "-enable", id)
		if err != nil {
			return err
		}
	}
	_, err := e2eutil.Command("nomad", "system", "gc")
	return err
}

func nodesForJob(jobID string) ([]string, error) {
	allocs, err := e2eutil.AllocsForJob(jobID, ns)
	if err != nil {
		return nil, err
	}
	if len(allocs) < 1 {
		return nil, fmt.Errorf("no allocs found for job: %v", jobID)
	}
	nodes := []string{}
	for _, alloc := range allocs {
		nodes = append(nodes, alloc["Node ID"])
	}
	return nodes, nil
}

// waitForNodeDrain is a convenience wrapper that polls 'node status'
// until the comparison function over the state of the job's allocs on that
// node returns true
func waitForNodeDrain(nodeID string, comparison func([]map[string]string) bool, wc *e2eutil.WaitConfig) error {
	var got []map[string]string
	var err error
	interval, retries := wc.OrDefault()
	testutil.WaitForResultRetries(retries, func() (bool, error) {
		time.Sleep(interval)
		got, err = e2eutil.AllocsForNode(nodeID)
		if err != nil {
			return false, err
		}
		return comparison(got), nil
	}, func(e error) {
		err = fmt.Errorf("node drain status check failed: %v\n%#v", e, got)
	})
	return err
}
