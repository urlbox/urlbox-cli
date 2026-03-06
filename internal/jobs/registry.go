package jobs

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/urlbox/cli/internal/config"
)

type Job struct {
	ID         int                    `json:"id"`
	RenderID   string                 `json:"render_id"`
	URL        string                 `json:"url,omitempty"`
	OutputPath string                 `json:"output_path,omitempty"`
	Status     string                 `json:"status,omitempty"`
	CreatedAt  string                 `json:"created_at"`
	UpdatedAt  string                 `json:"updated_at"`
	Result     map[string]interface{} `json:"result,omitempty"`
}

type File struct {
	NextID int   `json:"next_id"`
	Jobs   []Job `json:"jobs"`
}

func path() string {
	return filepath.Join(filepath.Dir(config.DefaultPath()), "jobs.json")
}

func Read() (File, error) {
	bytes, err := ioutil.ReadFile(path())
	if os.IsNotExist(err) {
		return File{NextID: 1, Jobs: []Job{}}, nil
	}
	if err != nil {
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(bytes, &file); err != nil {
		return File{}, err
	}
	if file.NextID == 0 {
		file.NextID = 1
	}
	return file, nil
}

func Write(file File) error {
	if err := os.MkdirAll(filepath.Dir(path()), 0755); err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path(), append(bytes, '\n'), 0644)
}

func Add(renderID string, url string, outputPath string) (Job, error) {
	file, err := Read()
	if err != nil {
		return Job{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	job := Job{
		ID:         file.NextID,
		RenderID:   renderID,
		URL:        url,
		OutputPath: outputPath,
		Status:     "created",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	file.NextID++
	file.Jobs = append(file.Jobs, job)
	if err := Write(file); err != nil {
		return Job{}, err
	}
	return job, nil
}

func Update(updated Job) error {
	file, err := Read()
	if err != nil {
		return err
	}
	for index, job := range file.Jobs {
		if job.ID == updated.ID {
			updated.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			file.Jobs[index] = updated
			return Write(file)
		}
	}
	return nil
}

func Find(id int) (Job, bool, error) {
	file, err := Read()
	if err != nil {
		return Job{}, false, err
	}
	for _, job := range file.Jobs {
		if job.ID == id {
			return job, true, nil
		}
	}
	return Job{}, false, nil
}
