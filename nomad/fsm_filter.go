package nomad

import (
	"fmt"
	"io"

	"github.com/hashicorp/go-msgpack/codec"

	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
)

type FSMFilter struct {
	Jobs  []string
	Nodes []string
}

func (f *FSMFilter) Include(filterSet []string, id string) bool {
	for _, filteredID := range filterSet {
		if filteredID == id {
			return true
		}
	}
	return false
}

func (n *nomadFSM) RestoreFiltered(old io.ReadCloser, filter *FSMFilter) error {
	defer old.Close()

	// Create a new state store
	config := &state.StateStoreConfig{
		Logger:          n.config.Logger,
		Region:          n.config.Region,
		EnablePublisher: n.config.EnableEventBroker,
		EventBufferSize: n.config.EventBufferSize,
	}
	newState, err := state.NewStateStore(config)
	if err != nil {
		return err
	}

	// Start the state restore
	restore, err := newState.Restore()
	if err != nil {
		return err
	}
	defer restore.Abort()

	// Create a decoder
	dec := codec.NewDecoder(old, structs.MsgpackHandle)

	// Read in the header
	var header snapshotHeader
	if err := dec.Decode(&header); err != nil {
		return err
	}

	// Populate the new state
	msgType := make([]byte, 1)
	for {
		// Read the message type
		_, err := old.Read(msgType)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		// Decode
		snapType := SnapshotType(msgType[0])
		switch snapType {
		case TimeTableSnapshot:
			if err := n.timetable.Deserialize(dec); err != nil {
				return fmt.Errorf("time table deserialize failed: %v", err)
			}

		case NodeSnapshot:
			node := new(structs.Node)
			if err := dec.Decode(node); err != nil {
				return err
			}
			if filter.Include(filter.Nodes, node.ID) {
				node.Canonicalize()
				if err := restore.NodeRestore(node); err != nil {
					return err
				}
			}

		case JobSnapshot:
			job := new(structs.Job)
			if err := dec.Decode(job); err != nil {
				return err
			}
			if filter.Include(filter.Jobs, job.ID) {
				job.Canonicalize()
				if err := restore.JobRestore(job); err != nil {
					return err
				}
			}

		case EvalSnapshot:
			eval := new(structs.Evaluation)
			if err := dec.Decode(eval); err != nil {
				return err
			}
			if filter.Include(filter.Jobs, eval.JobID) {
				if err := restore.EvalRestore(eval); err != nil {
					return err
				}
			}

		case AllocSnapshot:
			alloc := new(structs.Allocation)
			if err := dec.Decode(alloc); err != nil {
				return err
			}
			if filter.Include(filter.Jobs, alloc.JobID) ||
				filter.Include(filter.Nodes, alloc.NodeID) {
				if err := restore.AllocRestore(alloc); err != nil {
					return err
				}
			}
		case IndexSnapshot:
			idx := new(state.IndexEntry)
			if err := dec.Decode(idx); err != nil {
				return err
			}
			if err := restore.IndexRestore(idx); err != nil {
				return err
			}

		case PeriodicLaunchSnapshot:
			launch := new(structs.PeriodicLaunch)
			if err := dec.Decode(launch); err != nil {
				return err
			}

		case JobSummarySnapshot:
			summary := new(structs.JobSummary)
			if err := dec.Decode(summary); err != nil {
				return err
			}
			if filter.Include(filter.Jobs, summary.JobID) {
				if err := restore.JobSummaryRestore(summary); err != nil {
					return err
				}
			}
		case VaultAccessorSnapshot:
			accessor := new(structs.VaultAccessor)
			if err := dec.Decode(accessor); err != nil {
				return err
			}

		case ServiceIdentityTokenAccessorSnapshot:
			accessor := new(structs.SITokenAccessor)
			if err := dec.Decode(accessor); err != nil {
				return err
			}

		case JobVersionSnapshot:
			version := new(structs.Job)
			if err := dec.Decode(version); err != nil {
				return err
			}
			if filter.Include(filter.Jobs, version.ID) {
				if err := restore.JobVersionRestore(version); err != nil {
					return err
				}
			}
		case DeploymentSnapshot:
			deployment := new(structs.Deployment)
			if err := dec.Decode(deployment); err != nil {
				return err
			}
			if filter.Include(filter.Jobs, deployment.JobID) {
				if err := restore.DeploymentRestore(deployment); err != nil {
					return err
				}
			}
		case ACLPolicySnapshot:
			policy := new(structs.ACLPolicy)
			if err := dec.Decode(policy); err != nil {
				return err
			}

		case ACLTokenSnapshot:
			token := new(structs.ACLToken)
			if err := dec.Decode(token); err != nil {
				return err
			}

		case SchedulerConfigSnapshot:
			schedConfig := new(structs.SchedulerConfiguration)
			if err := dec.Decode(schedConfig); err != nil {
				return err
			}

		case ClusterMetadataSnapshot:
			meta := new(structs.ClusterMetadata)
			if err := dec.Decode(meta); err != nil {
				return err
			}

		case ScalingEventsSnapshot:
			jobScalingEvents := new(structs.JobScalingEvents)
			if err := dec.Decode(jobScalingEvents); err != nil {
				return err
			}

		case ScalingPolicySnapshot:
			scalingPolicy := new(structs.ScalingPolicy)
			if err := dec.Decode(scalingPolicy); err != nil {
				return err
			}

		case CSIPluginSnapshot:
			plugin := new(structs.CSIPlugin)
			if err := dec.Decode(plugin); err != nil {
				return err
			}

		case CSIVolumeSnapshot:
			plugin := new(structs.CSIVolume)
			if err := dec.Decode(plugin); err != nil {
				return err
			}

		case NamespaceSnapshot:
			namespace := new(structs.Namespace)
			if err := dec.Decode(namespace); err != nil {
				return err
			}
			if err := restore.NamespaceRestore(namespace); err != nil {
				return err
			}

		default:
			// Check if this is an enterprise only object being restored
			restorer, ok := n.enterpriseRestorers[snapType]
			if !ok {
				return fmt.Errorf("Unrecognized snapshot type: %v", msgType)
			}

			// Restore the enterprise only object
			if err := restorer(restore, dec); err != nil {
				return err
			}
		}
	}

	if err := restore.Commit(); err != nil {
		return err
	}

	// External code might be calling State(), so we need to synchronize
	// here to make sure we swap in the new state store atomically.
	n.stateLock.Lock()
	stateOld := n.state
	n.state = newState
	n.stateLock.Unlock()

	// Signal that the old state store has been abandoned. This is required
	// because we don't operate on it any more, we just throw it away, so
	// blocking queries won't see any changes and need to be woken up.
	stateOld.Abandon()

	return nil
}
