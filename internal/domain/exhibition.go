package domain

import "time"

type DisplayCaseStatus string

const (
	CaseAvailable DisplayCaseStatus = "available"
	CaseReserved  DisplayCaseStatus = "reserved"
	CaseActive    DisplayCaseStatus = "active"
	CaseIncident  DisplayCaseStatus = "incident"
	CaseOffline   DisplayCaseStatus = "offline"
)

type DisplayCase struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	Gallery       string            `json:"gallery"`
	Name          string            `json:"name"`
	Status        DisplayCaseStatus `json:"status"`
	ArtifactID    string            `json:"artifact_id,omitempty"`
	MinHumidity   float64           `json:"min_humidity"`
	MaxHumidity   float64           `json:"max_humidity"`
	MinTempC      float64           `json:"min_temp_c"`
	MaxTempC      float64           `json:"max_temp_c"`
	ReservationTo *time.Time        `json:"reservation_to,omitempty"`
	Version       int64             `json:"version"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type Installation struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	ArtifactID       string    `json:"artifact_id"`
	DisplayCaseID    string    `json:"display_case_id"`
	MountVerified    bool      `json:"mount_verified"`
	SealVerified     bool      `json:"seal_verified"`
	EnvironmentReady bool      `json:"environment_ready"`
	InstalledBy      string    `json:"installed_by"`
	InstalledAt      time.Time `json:"installed_at"`
}

func (i Installation) Complete() bool {
	return i.MountVerified && i.SealVerified && i.EnvironmentReady
}

type EnvironmentReading struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	DisplayCaseID string    `json:"display_case_id"`
	DeviceID      string    `json:"device_id"`
	Sequence      int64     `json:"sequence"`
	TemperatureC  float64   `json:"temperature_c"`
	Humidity      float64   `json:"humidity"`
	ObservedAt    time.Time `json:"observed_at"`
	ReceivedAt    time.Time `json:"received_at"`
}

type EnvironmentAssessment struct {
	DisplayCaseID string    `json:"display_case_id"`
	ReadingCount  int       `json:"reading_count"`
	Ready         bool      `json:"ready"`
	Reasons       []string  `json:"reasons"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
}
