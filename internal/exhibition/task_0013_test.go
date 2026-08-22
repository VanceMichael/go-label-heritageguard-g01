package exhibition

import (
	"context"
	"testing"

)

func TestHeritageGuardTask0013(t *testing.T) {
	store := exhibitionStore(t)
	service := exhibitionService(store)
	reading := ReadingInput{DisplayCaseID: "case-east-01", DeviceID: "device-task-0013", Sequence: 7, TemperatureC: 20, Humidity: 49, ObservedAt: store.Now()}
	if _, err := service.RecordReading(context.Background(), "museum-demo", reading); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordReading(context.Background(), "museum-demo", reading); err == nil {
		t.Fatal("replayed sensor sequence must be rejected")
	}
	var readings, jobs int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM environment_readings WHERE device_id = 'device-task-0013' AND sequence = 7`).Scan(&readings); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM worker_jobs WHERE aggregate_id = 'case-east-01' AND kind = ?`, EnvironmentAssessmentJob).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if readings != 1 || jobs != 1 {
		t.Fatalf("sensor replay did not converge: readings=%d jobs=%d", readings, jobs)
	}
}
