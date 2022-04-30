package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"

	concourse "github.com/cludden/concourse-go-sdk"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

func TestResource(t *testing.T) {
	dir := t.TempDir()

	if !assert.NoError(t, os.Mkdir(path.Join(dir, "repo"), os.ModePerm)) {
		t.FailNow()
	}

	if !assert.NoError(t, ioutil.WriteFile(path.Join(dir, "repo", "ref"), []byte("5541858611c514f02fd7e3f34d3fcad17908d933"), 0777)) {
		t.FailNow()
	}

	os.Setenv("BUILD_ID", "1234")
	os.Setenv("BUILD_NAME", "1")
	os.Setenv("BUILD_JOB_NAME", "first")
	os.Setenv("BUILD_PIPELINE_NAME", "test")
	os.Setenv("BUILD_TEAM_NAME", "main")
	os.Setenv("ATC_EXTERNAL_URL", "https://concourse.example.com")

	putInput, err := json.Marshal(map[string]interface{}{
		"source": nil,
		"params": &PutParams{
			Mapping: `
			id = ksuid()
			ref = file("repo/ref").string().uppercase()
			url = build_url
			`,
		},
	})
	if err != nil {
		t.Fatalf("error serializing put input: %v", err)
	}

	out := &bytes.Buffer{}
	if !assert.NoError(t, concourse.Exec(context.Background(), concourse.OutOp, &Resource{}, bytes.NewBuffer(putInput), out, os.Stderr, []string{"/opt/resource/out", dir})) {
		t.FailNow()
	}

	var putOutput struct {
		Version Version `json:"version"`
	}
	if !assert.NoError(t, json.Unmarshal(out.Bytes(), &putOutput)) {
		t.FailNow()
	}

	getInput, err := json.Marshal(map[string]interface{}{
		"source":  nil,
		"version": &putOutput.Version,
		"params": map[string]interface{}{
			"files": map[string]interface{}{
				"test.yml": `root = this.format_yaml()`,
			},
		},
	})
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	if !assert.NoError(t, concourse.Exec(context.Background(), concourse.InOp, &Resource{}, bytes.NewBuffer(getInput), io.Discard, os.Stderr, []string{"/opt/resource/in", dir})) {
		t.FailNow()
	}

	if p := path.Join(dir, "version.json"); assert.FileExists(t, p) {
		b, err := ioutil.ReadFile(p)
		if err != nil {
			t.Fatalf("error reading version.json: %v", err)
		}
		versionData := make(map[string]interface{})
		if err := json.Unmarshal(b, &versionData); err != nil {
			t.Fatalf("error unmarshalling version.json: %v", err)
		}
		assert.Len(t, versionData["id"].(string), 27)
		assert.Equal(t, "5541858611C514F02FD7E3F34D3FCAD17908D933", versionData["ref"].(string))
		assert.Equal(t, "https://concourse.example.com/builds/1234", versionData["url"].(string))
	}

	if p := path.Join(dir, "test.yml"); assert.FileExists(t, p) {
		b, err := ioutil.ReadFile(p)
		if err != nil {
			t.Fatalf("error reading test.yml: %v", err)
		}
		var versionData struct {
			BuildID string `yaml:"build_id"`
			ID      string `yaml:"id"`
			Ref     string `yaml:"ref"`
			URL     string `yaml:"url"`
		}
		if err := yaml.Unmarshal(b, &versionData); err != nil {
			t.Fatalf("error unmarshalling test.yml: %v", err)
		}
		assert.Len(t, versionData.ID, 27)
		assert.Equal(t, "1234", versionData.BuildID)
		assert.Equal(t, "5541858611C514F02FD7E3F34D3FCAD17908D933", versionData.Ref)
		assert.Equal(t, "https://concourse.example.com/builds/1234", versionData.URL)
	}
}
