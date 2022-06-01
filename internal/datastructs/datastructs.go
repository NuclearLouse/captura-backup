package datastructs

import "time"

type ControlPanel struct {
	TypeArchive       int
	StopService       bool
	StartRecovery     bool
	ReloadConfig      bool
	StartManualBackup bool
	RestoreToThisDate time.Time
}

type ArchiveTable struct {
	ID              int
	Name            string
	Entity          string
	DateColumn      string
	ConditionFmt    string
	RmInterval      string
	RestoreTemplate string
	DoBackup        bool
	KeepRestore     int
}

type ArchivingOptions struct {
	ID                   int
	ArchDataAfter        int
	KeepArchDays         int
	KeepRestoreData      int
	Schema               string
	TblNamePattern       string
	ArchIntervalName     string
	Entity               string
	ConditionFmt         string
	DateColumn           string
	TblNameDateFmtSbstr  string
	TblNameDateFmtToDate string
	RestoreTemplate      string
}

type ArchAvailableData struct {
	ID              int
	DataID          int
	ContentRows     int64
	SchemaName      string
	TableName       string
	FileName        string
	// Comment         string
	RestoreTemplate string
	SingleTable     bool
	ContentDate     time.Time
	ArchivedAt      time.Time
	RestoredAt      time.Time
	DeletedAt       time.Time
}

type RestoreData struct {
	ArchAvailableData
	CurrentTemplate string
}

type ScheduleConfig struct {
	DataID            int
	CheckInterval     string
	CheckIntervalDays []int
	WorkStart         time.Time
}

func DefaultScheduleConfig() []*ScheduleConfig {
	return []*ScheduleConfig{
		{
			DataID:            1,
			CheckInterval:     "WEEK",
			CheckIntervalDays: []int{7},
		},
		{
			DataID:            2,
			CheckInterval:     "WEEK",
			CheckIntervalDays: []int{7},
		},
		{
			DataID:            3,
			CheckInterval:     "MONTH",
			CheckIntervalDays: []int{1},
		},
		{
			DataID:            4,
			CheckInterval:     "MONTH",
			CheckIntervalDays: []int{1, 15},
		},
		{
			DataID:            5,
			CheckInterval:     "WEEK",
			CheckIntervalDays: []int{7},
		},
		{
			DataID:            6,
			CheckInterval:     "WEEK",
			CheckIntervalDays: []int{1, 2, 3, 4, 5, 6, 7},
		},
	}
}

type ArchiveStorage struct {
	DataID       int
	Schemaname   string
	KeepArchDays int
}

