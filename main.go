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
	"github.com/cludden/concourse-go-sdk/pkg/archive"
	"github.com/hashicorp/go-multierror"
	"gopkg.in/yaml.v2"
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
		Archive        *archive.Config `json:"archive"`
		InitialMapping string          `json:"initial_mapping"`
	}

	// Version holds arbitrary key value data that can be passed across
	// jobs within a pipeline
	Version struct {
		Data map[string]interface{}
	}
)

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

// Archive initializes a new archive value if configured
func (r *Resource) Archive(ctx context.Context, s *Source) (archive.Archive, error) {
	if s != nil && s.Archive != nil {
		return archive.New(ctx, *s.Archive)
	}
	return nil, nil
}

// Check is a required resource method, but is a no-op for this resource
func (r *Resource) Check(ctx context.Context, s *Source, v *Version) (versions []Version, err error) {
	if v != nil {
		versions = append(versions, *v)
	}
	if len(versions) == 0 && s != nil && s.InitialMapping != "" {
		init, _, err := r.newVersion(ctx, s.InitialMapping)
		if err != nil {
			return nil, err
		}
		versions = append(versions, *init)
	}
	return
}

// In writes an incoming version the filesystem, allowing downstream steams to utilize
// arbitary data from an earlier put step
func (r *Resource) In(ctx context.Context, s *Source, v *Version, dir string, p *GetParams) ([]concourse.Metadata, error) {
	if err := writeJSON(dir, "version.json", v); err != nil {
		return nil, fmt.Errorf("error writing version.json: %v", err)
	}

	doc, meta := metadata()
	if err := writeJSON(dir, "metadata.json", doc); err != nil {
		return nil, fmt.Errorf("error writing metadata.json: %v", err)
	}

	if p != nil && len(p.Files) > 0 {
		for k, v := range v.Data {
			doc[k] = v
		}
		for f, m := range p.Files {
			e, err := bloblang.Parse(m)
			if err != nil {
				return nil, fmt.Errorf("error parsing '%s' file mapping: %v", f, err)
			}

			raw, err := e.Query(doc)
			if err != nil {
				return nil, fmt.Errorf("error executing '%s' file mapping: %v", f, err)
			}

			var b []byte
			switch v := raw.(type) {
			case string:
				b = []byte(v)
			case []byte:
				b = v
			default:
				switch path.Ext(f) {
				case ".json":
					b, err = json.Marshal(raw)
					if err != nil {
						return nil, fmt.Errorf("error serializing '%s' file mapping result (%T) as json: %v", f, raw, err)
					}
				case ".yaml", ".yml":
					b, err = yaml.Marshal(raw)
					if err != nil {
						return nil, fmt.Errorf("error serializing '%s' file mapping result (%T) as yaml: %v", f, raw, err)
					}
				default:
					return nil, fmt.Errorf("unclear how to serialize result (%T) returned by '%s' file mapping: try adding a supported file extension (.json, .yml)", raw, f)
				}
			}

			if err := ioutil.WriteFile(path.Join(dir, f), b, 0777); err != nil {
				return nil, fmt.Errorf("error writing '%s' file: %v", f, err)
			}
		}
	}

	return meta, nil
}

// Out generates a new version that contains arbitray key value pairs, where both keys
// and values are string data
func (r *Resource) Out(ctx context.Context, s *Source, dir string, p *PutParams) (*Version, []concourse.Metadata, error) {
	m := "root = this"
	if p != nil && p.Mapping != "" {
		m = p.Mapping
	}
	return r.newVersion(ctx, m)
}

// =============================================================================

// metadata returns build metadata as a map[string]interface{} to be used as the input
// document to bloblang mappings, in addition to a []concourse.Metadata to be returned
// by In and Out methods
func metadata() (doc map[string]interface{}, meta []concourse.Metadata) {
	doc = map[string]interface{}{
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
	for k, v := range doc {
		meta = append(meta, concourse.Metadata{
			Name:  k,
			Value: v.(string),
		})
	}
	return
}

// newVersion initializes a new Version value using the specified mapping
func (r *Resource) newVersion(ctx context.Context, mapping string) (*Version, []concourse.Metadata, error) {
	e, err := bloblang.Parse(mapping)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing 'mapping': %v", err)
	}

	doc, meta := metadata()

	raw, err := e.Query(doc)
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
	return &Version{Data: data}, meta, nil
}

// writeJSON creates a formatted json file at the given directory + path
func writeJSON(dir, filename string, data interface{}) error {
	f, err := os.Create(path.Join(dir, filename))
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
