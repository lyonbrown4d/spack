package pipeline

import (
	"cmp"
	cxlist "github.com/arcgolabs/collectionx/list"
)

type stageRegistration struct {
	Order int
	Stage Stage
}

func newStageRegistration(order int, stage Stage) stageRegistration {
	return stageRegistration{
		Order: order,
		Stage: stage,
	}
}

func buildStages(registrations *cxlist.List[stageRegistration]) *cxlist.List[Stage] {
	if registrations.IsEmpty() {
		return cxlist.NewList[Stage]()
	}

	sorted := registrations.Clone().Sort(func(left, right stageRegistration) int {
		if left.Order != right.Order {
			return cmp.Compare(left.Order, right.Order)
		}
		switch {
		case left.Stage == nil && right.Stage == nil:
			return 0
		case left.Stage == nil:
			return 1
		case right.Stage == nil:
			return -1
		default:
			return compareStageNames(left.Stage, right.Stage)
		}
	})

	stages := cxlist.NewListWithCapacity[Stage](sorted.Len())
	sorted.Range(func(_ int, registration stageRegistration) bool {
		if registration.Stage == nil {
			return true
		}
		stages.Add(registration.Stage)
		return true
	})
	return stages
}

func compareStageNames(left, right Stage) int {
	return cmp.Compare(left.Name(), right.Name())
}
