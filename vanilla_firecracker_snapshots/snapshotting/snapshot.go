package snapshotting

import (
	"path/filepath"
)

// Snapshot identified by revision
type Snapshot struct {
	id           string // Snapshot identified by K_REVISION env variable (eg. helloworld-go-00001)
	baseFolder   string
	img          string
	totalSizeMiB int
}

func NewSnapshot(id, baseFolder string) Snapshot {
	return Snapshot{id: id, baseFolder: filepath.Join(baseFolder, id)}
}

func (snp *Snapshot) GetBaseFolder() string {
	return snp.baseFolder
}

func (snp *Snapshot) GetSnapFilePath() string {
	return filepath.Join(snp.baseFolder, "snapfile")
}

func (snp *Snapshot) GetMemFilePath() string {
	return filepath.Join(snp.baseFolder, "memfile")
}

func (snp *Snapshot) GetCtrSnapCommitName() string {
	return snp.img+"-commit-"+snp.id
}

func (snp *Snapshot) GetInfoFilePath() string {
	return filepath.Join(snp.baseFolder, "infofile")
}
