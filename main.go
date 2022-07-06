package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/benthosdev/benthos/v4/public/bloblang"
	concourse "github.com/cludden/concourse-go-sdk"
	"github.com/hashicorp/go-multierror"
)

const (
	defaultInitialMapping = "id = ksuid()"
	defualtMapping        = "root = this"
)

func main() {
	concourse.Main(&Resource{})
}

// =============================================================================

type (
	// GetParams describes the available parameters for a get step
	GetParams struct {
		Files map[string]string `json:"files"`
	}

	// PutParams describes the available parameters for a put step
	PutParams struct {
		Mapping string `json:"mapping"`
	}

	// Source describes resource configuration
	Source struct {
		InitialMapping string `json:"initial_mapping"`
	}

	// Version holds arbitrary key value data that can be passed across
	// jobs within a pipeline
	Version struct {
		Data map[string]interface{}
	}
)

func (s *Source) buildDocument() map[string]interface{} {
	doc := map[string]interface{}{
		"build_id":       os.Getenv("BUILD_ID"),
		"build_name":     os.Getenv("BUILD_NAME"),
		"build_job":      os.Getenv("BUILD_JOB_NAME"),
		"build_pipeline": os.Getenv("BUILD_PIPELINE_NAME"),
		"build_team":     os.Getenv("BUILD_TEAM_NAME"),
		"build_url":      fmt.Sprintf("%s/builds/%s", os.Getenv("ATC_EXTERNAL_URL"), os.Getenv("BUILD_ID")),
	}
	if entry := os.Getenv("BUILD_CREATED_BY"); entry != "" {
		doc["build_created_by"] = entry
	}
	if entry := os.Getenv("BUILD_PIPELINE_INSTANCE_VARS"); entry != "" {
		doc["build_instance_vars"] = entry
	}
	return doc
}

func (v *Version) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.Data)
}

func (v *Version) UnmarshalJSON(b []byte) error {
	v.Data = make(map[string]interface{})
	return json.Unmarshal(b, &v.Data)
}

// =============================================================================

// Resource implements a keyval concourse resource
type Resource struct{}

// Check is a required resource method, but is a no-op for this resource
func (r *Resource) Check(ctx context.Context, s *Source, v *Version) (versions []Version, err error) {
	if v == nil {
		m := defaultInitialMapping
		if s != nil && s.InitialMapping != "" {
			m = s.InitialMapping
		}
		init, _, err := r.newVersion(ctx, s, m)
		if err != nil {
			return nil, err
		}
		versions = append(versions, *init)
	} else {
		versions = append(versions, *v)
	}
	return
}

// In writes an incoming version the filesystem, allowing downstream steams to utilize
// arbitary data from an earlier put step
func (r *Resource) In(ctx context.Context, s *Source, v *Version, dir string, p *GetParams) (*Version, []concourse.Metadata, error) {
	version, err := os.Create(path.Join(dir, "version.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating version.json: %v", err)
	}

	venc := json.NewEncoder(version)
	venc.SetIndent("", "  ")
	if err := venc.Encode(v.Data); err != nil {
		return nil, nil, fmt.Errorf("error writing version.json: %v", err)
	}

	metadata, err := os.Create(path.Join(dir, "metadata.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("error creating metadata.json: %v", err)
	}
	menc := json.NewEncoder(metadata)
	menc.SetIndent("", "  ")
	if err := menc.Encode(s.buildDocument()); err != nil {
		return nil, nil, fmt.Errorf("error writing metadata.json: %v", err)
	}

	if p != nil && len(p.Files) > 0 {
		buildDoc := s.buildDocument()
		for k, v := range v.Data {
			buildDoc[k] = v
		}
		for f, m := range p.Files {
			e, err := bloblang.Parse(m)
			if err != nil {
				return nil, nil, fmt.Errorf("error parsing '%s' file mapping: %v", f, err)
			}

			raw, err := e.Query(buildDoc)
			if err != nil {
				return nil, nil, fmt.Errorf("error executing '%s' file mapping: %v", f, err)
			}

			var b []byte
			switch v := raw.(type) {
			case string:
				b = []byte(v)
			case []byte:
				b = v
			default:
				return nil, nil, fmt.Errorf("invalid type returned by '%s' file mapping: %v", f, v)
			}

			if err := ioutil.WriteFile(path.Join(dir, f), b, 0777); err != nil {
				return nil, nil, fmt.Errorf("error writing '%s' file: %v", f, err)
			}
		}
	}

	return v, nil, nil
}

// Out generates a new version that contains arbitray key value pairs, where both keys
// and values are string data
func (r *Resource) Out(ctx context.Context, s *Source, dir string, p *PutParams) (*Version, []concourse.Metadata, error) {
	m := defualtMapping
	if p != nil && p.Mapping != "" {
		m = p.Mapping
	}
	return r.newVersion(ctx, s, m)
}

// =============================================================================

func (r *Resource) newVersion(ctx context.Context, s *Source, m string) (*Version, []concourse.Metadata, error) {
	e, err := bloblang.Parse(m)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing 'mapping': %v", err)
	}

	meta := s.buildDocument()
	metadata := []concourse.Metadata{}
	for k, raw := range meta {
		if v, ok := raw.(string); ok {
			metadata = append(metadata, concourse.Metadata{
				Name:  k,
				Value: v,
			})
		}
	}

	raw, err := e.Query(meta)
	if err != nil {
		return nil, nil, fmt.Errorf("error executing version mapping: %v", err)
	}

	data, ok := raw.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("version mapping returned invalid result, expected map, got: %T", raw)
	}

	errs := multierror.Append(nil)
	for k, v := range data {
		if _, ok := v.(string); !ok {
			err = multierror.Append(err, fmt.Errorf("invalid version key '%s', expected string value, got: %T", k, v))
		}
	}
	if errs.Len() > 0 {
		return nil, nil, fmt.Errorf("version mapping returned invalid result: %s", errs.Error())
	}
	return &Version{Data: data}, metadata, nil
}
