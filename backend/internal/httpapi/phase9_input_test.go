package httpapi

import (
	"encoding/json"
	"testing"
)

func TestRoutineMembershipTriStateAndArchiveDestinationPresence(t *testing.T) {
	var assignment assignmentBody
	if err := json.Unmarshal([]byte(`{"effectiveDate":"2026-08-20"}`), &assignment); err != nil {
		t.Fatal(err)
	}
	if assignment.routineGroupSet {
		t.Fatal("omitted assignment routine group must preserve membership")
	}
	if err := json.Unmarshal([]byte(`{"routineGroupId":null,"effectiveDate":"2026-08-20"}`), &assignment); err != nil {
		t.Fatal(err)
	}
	if !assignment.routineGroupSet || assignment.RoutineGroupID != nil {
		t.Fatal("explicit null assignment routine group must mean Other")
	}
	var task taskBody
	if err := json.Unmarshal([]byte(`{"title":"same"}`), &task); err != nil {
		t.Fatal(err)
	}
	if task.routineGroupSet {
		t.Fatal("omitted task routine group must preserve membership")
	}
	if err := json.Unmarshal([]byte(`{"routineGroupId":null}`), &task); err != nil {
		t.Fatal(err)
	}
	if !task.routineGroupSet || task.RoutineGroupID != nil {
		t.Fatal("explicit null task routine group must mean Other")
	}
	var archive archiveRoutineBody
	if err := json.Unmarshal([]byte(`{"effectiveFrom":"2026-08-20"}`), &archive); err != nil {
		t.Fatal(err)
	}
	if archive.destinationSet {
		t.Fatal("omitted archive destination must be rejected")
	}
	if err := json.Unmarshal([]byte(`{"effectiveFrom":"2026-08-20","moveToRoutineGroupId":null}`), &archive); err != nil {
		t.Fatal(err)
	}
	if !archive.destinationSet || archive.MoveToRoutineGroupID != nil {
		t.Fatal("explicit null archive destination must mean Other")
	}
}
