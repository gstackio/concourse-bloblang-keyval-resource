package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"testing"

	"github.com/benthosdev/benthos/v4/public/bloblang"
	sdk "github.com/cludden/concourse-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestBloblang(t *testing.T) {
	var methods, functions []string
	env := bloblang.NewEnvironment()
	env.WalkFunctions(func(name string, _ *bloblang.FunctionView) {
		functions = append(functions, name)
	})
	env.WalkMethods(func(name string, _ *bloblang.MethodView) {
		methods = append(methods, name)
	})
	assert.Contains(t, functions, "file")
}

func TestResource(t *testing.T) {
	require := require.New(t)

	// prepare temp working directory
	dir := t.TempDir()
	require.NoError(os.Mkdir(path.Join(dir, "repo"), os.ModePerm))
	require.NoError(os.WriteFile(path.Join(dir, "repo", "ref"), []byte("5541858611c514f02fd7e3f34d3fcad17908d933"), 0777))

	// prepare environment variables
	os.Setenv("BUILD_ID", "1234")
	os.Setenv("BUILD_NAME", "1")
	os.Setenv("BUILD_JOB_NAME", "first")
	os.Setenv("BUILD_PIPELINE_NAME", "test")
	os.Setenv("BUILD_TEAM_NAME", "main")
	os.Setenv("ATC_EXTERNAL_URL", "https://concourse.example.com")

	// prepare put payload
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
	require.NoError(err)

	// execute put step
	out := &bytes.Buffer{}
	require.NoError(sdk.Exec[Source, Version, GetParams, PutParams](
		context.Background(), sdk.OutOp, &Resource{}, bytes.NewBuffer(putInput), out, os.Stderr, []string{"/opt/resource/out", dir},
	))

	// unmarshal version
	var putOutput struct {
		Version Version `json:"version"`
	}
	require.NoError(json.Unmarshal(out.Bytes(), &putOutput))

	// prepare get payload
	getInput, err := json.Marshal(map[string]interface{}{
		"source":  nil,
		"version": &putOutput.Version,
		"params": map[string]interface{}{
			"files": map[string]interface{}{
				"test.yml": `root = this.format_yaml()`,
			},
		},
	})
	require.NoError(err)

	// execute get step
	require.NoError(sdk.Exec[Source, Version, GetParams, PutParams](
		context.Background(), sdk.InOp, &Resource{}, bytes.NewBuffer(getInput), io.Discard, os.Stderr, []string{"/opt/resource/in", dir},
	))

	// verify version.json
	p := path.Join(dir, "version.json")
	require.FileExists(p)
	b, err := os.ReadFile(p)
	require.NoError(err)
	versionData := make(map[string]interface{})
	require.NoError(json.Unmarshal(b, &versionData))
	require.Len(versionData["id"].(string), 27)
	require.Equal("5541858611C514F02FD7E3F34D3FCAD17908D933", versionData["ref"].(string))
	require.Equal("https://concourse.example.com/builds/1234", versionData["url"].(string))

	// verify custom get files
	p = path.Join(dir, "test.yml")
	require.FileExists(p)
	b, err = os.ReadFile(p)
	require.NoError(err)
	var testData struct {
		BuildID string `yaml:"build_id"`
		ID      string `yaml:"id"`
		Ref     string `yaml:"ref"`
		URL     string `yaml:"url"`
	}
	require.NoError(yaml.Unmarshal(b, &testData))
	require.Len(testData.ID, 27)
	require.Equal("1234", testData.BuildID)
	require.Equal("5541858611C514F02FD7E3F34D3FCAD17908D933", testData.Ref)
	require.Equal("https://concourse.example.com/builds/1234", testData.URL)
}
