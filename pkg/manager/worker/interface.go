package worker

import (
	"github.com/kubeflow/katib/pkg/api"
)

type Interface interface {
	SpawnWorker(wid string, workerConf *api.WorkerConfig) error
	StoreWorkerLog(wID string, objectiveValueName string, metrics []string) error
	IsWorkerComplete(wID string) (bool, error)
	UpdateWorkerStatus(studyId string, objectiveValueName string, metrics []string) error
	StopWorkers(studyId string, wIDs []string, iscomplete bool) error
	CleanWorkers(studyId string) error
}
