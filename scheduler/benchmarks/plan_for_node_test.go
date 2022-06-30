package benchmarks

import (
	"fmt"
	"testing"

	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/scheduler"
	"github.com/stretchr/testify/require"
)

func TestAllocFitComparison(t *testing.T) {

	fmt.Println("starting snapshot load")
	h, err := NewHarnessFromSnapshot(t, "/Users/timgross/ws/nomad/doc/private/roblox/raft-is-stuck/stress-testing-before/35.153.198.152-2021-11-17T17_41_58-0500/backup.snap")

	///Users/timgross/ws/nomad/doc/private/plan-for-node-rejected/nomad_operator_snapshot_save_2022_05_12_1543-0700.snap

	require.NoError(t, err)
	fmt.Println("done!")

	node := mock.Node()
	require.Equal(t, []structs.Port{{Label: "ssh", Value: 22}},
		node.Reserved.Networks[0].ReservedPorts)
	require.NoError(t, h.State.UpsertNode(structs.MsgTypeTestSetup, h.NextIndex(), node))

	// system job
	job := mock.SystemJob()
	job.Status = structs.JobStatusRunning
	job.TaskGroups[0].Tasks[0].Resources.Networks = nil
	job.TaskGroups[0].Networks = []*structs.NetworkResource{
		{
			Mode: "host",
			ReservedPorts: []structs.Port{
				{
					Label:       "foo",
					Value:       6831,
					HostNetwork: "default",
				},
				{
					Label:       "bar",
					Value:       9411,
					HostNetwork: "default",
				},
			},
			DynamicPorts: []structs.Port{
				{
					Label:       "baz",
					Value:       26041,
					HostNetwork: "default",
				},
			},
		},
	}
	require.NoError(t, h.State.UpsertJob(structs.MsgTypeTestSetup, h.NextIndex(), job))

	// re-evaluate
	eval := &structs.Evaluation{
		Namespace:   structs.DefaultNamespace,
		ID:          uuid.Generate(),
		Priority:    job.Priority,
		TriggeredBy: structs.EvalTriggerJobRegister,
		JobID:       job.ID,
		Status:      structs.EvalStatusPending,
	}

	require.NoError(t, h.State.UpsertEvals(
		structs.MsgTypeTestSetup, h.NextIndex(), []*structs.Evaluation{eval}))

	// Process the evaluation
	err = h.Process(scheduler.NewSystemScheduler, eval)
	require.NoError(t, err)
	require.Len(t, h.Plans, 1)
	plan := h.Plans[0]

	require.Len(t, plan.NodeAllocation[node.ID], 1)
	alloc0 := plan.NodeAllocation[node.ID][0]
	require.NoError(t, h.State.UpsertAllocs(
		structs.MsgTypeTestSetup, h.NextIndex(), plan.NodeAllocation[node.ID]))

	// Update task states for alloc
	alloc0 = alloc0.Copy()
	alloc0.ClientStatus = structs.AllocClientStatusRunning
	alloc0.AllocStates = nil
	alloc0.PreviousAllocation = uuid.Generate()
	alloc0.ClientStatus = structs.AllocClientStatusFailed
	h.State.UpdateAllocsFromClient(
		structs.MsgTypeTestSetup, h.NextIndex(), []*structs.Allocation{alloc0})

	// re-evaluate the job
	eval.ID = uuid.Generate()
	require.NoError(t, h.State.UpsertEvals(
		structs.MsgTypeTestSetup, h.NextIndex(), []*structs.Evaluation{eval}))
	require.NoError(t, err)

	// Process and get an update-in-place plan
	err = h.Process(scheduler.NewSystemScheduler, eval)
	require.NoError(t, err)
	require.Len(t, h.Plans, 2)

	fmt.Printf("original    %s\n", alloc0.ID)

	require.Len(t, alloc0.Resources.Networks[0].ReservedPorts, 2)
	require.Len(t, alloc0.Resources.Networks[0].DynamicPorts, 1)

	require.Len(t, alloc0.SharedResources.Networks[0].ReservedPorts, 2)
	require.Len(t, alloc0.SharedResources.Networks[0].DynamicPorts, 1)

	require.Nil(t, alloc0.TaskResources["web"].Networks)
	require.Nil(t, alloc0.AllocatedResources.Tasks["web"].Networks)
	require.Len(t, alloc0.AllocatedResources.Shared.Networks[0].ReservedPorts, 2)
	require.Len(t, alloc0.AllocatedResources.Shared.Networks[0].DynamicPorts, 1)
	require.Len(t, alloc0.AllocatedResources.Shared.Ports, 3)

	// spew.Dump(alloc0.AllocatedResources)

	plan = h.Plans[1]
	require.Len(t, plan.NodeUpdate[node.ID], 0)
	require.Len(t, plan.NodeAllocation[node.ID], 1)

	newAlloc := plan.NodeAllocation[node.ID][0]
	fmt.Printf("replacement %s => %s\n", newAlloc.ID, newAlloc.DesiredStatus)

	//spew.Dump(newAlloc.AllocatedResources)
	fmt.Println("-----------")

	ok, reason, comparable, err := structs.AllocsFit(
		node, plan.NodeAllocation[node.ID], nil, false)
	require.True(t, ok)
	require.Equal(t, "", reason)
	require.Len(t, comparable.Flattened.Networks[0].ReservedPorts, 2)
	require.NoError(t, err)
}
