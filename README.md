# concourse-keyval-resource
a [Concourse](https://concourse-ci.org/resources.html) resource for:
- passing arbitrary data between jobs
- generating dynamic files
- extracting filesystem info

## Getting Started
```yaml
resource_types:
  - name: keyval
    type: registry-image
    source:
      repository: ghcr.io/cludden/concourse-keyval-resource

resources:
  - name: keyval
    type: keyval
    icon: table
    expose_build_created_by: true

jobs:
  - name: example
    plan:
      - get: keyval

      - put: keyval
        params:
          mapping: |
            root = this.map_each_key(k -> k.replace_all("build_", ""))
            id = ksuid()
            previous_id = file("keyval/version.json").parse_json().id
```

## Configuration

**Parameters:**
| Parameter | Type | Description | Required |
| :--- | :---: | :--- | :---: |
| initial_mapping | `string` | a [Bloblang mapping](https://www.benthos.dev/docs/guides/bloblang/about) that can be used to customize the initial version returned by this resource, defaults to `id = ksuid()` | |

## Behavior

### `check`
Check returns the latest available version produced by a `put` step. If `get` is used prior to `put`, an initial version will be returned which can be customized via the `initial_mapping` resource parameter.

### `in`
Fetches arbitrary key value data from a prior put step and writes it the file system as a JSON document.

**Parameters:**
| Parameter | Type | Description | Required |
| :--- | :---: | :--- | :---: |
| files | `map[string]string` | a map of filenames to [Bloblang mappings](https://www.benthos.dev/docs/guides/bloblang/about), where the input document contains the [build metadata](https://concourse-ci.org/implementing-resource-types.html#resource-metadata) along with any key value data from the fetched version, and the output is a `string` or `[]byte` that is the content of the file to write | |

**Files:**
- `version.json` - the key value pairs serialized as a JSON document
- `metadata.json` - the [build metadata](https://concourse-ci.org/implementing-resource-types.html#resource-metadata) serialized as a JSON document
- `*` any file mappings defined in the `files` parameter

### `out`: publish arbitrary key value data
Publishes arbitrary key value data to be shared across jobs.

**Parameters:**
| Parameter | Type | Description | Required |
| :--- | :---: | :--- | :---: |
| mapping | `string` | a [Bloblang mappings](https://www.benthos.dev/docs/guides/bloblang/about), where the input document contains the [build metadata](https://concourse-ci.org/implementing-resource-types.html#resource-metadata), and the output is a `map[string]string`, defaults to `root = this` | |

## Build Metadata
Each [Bloblang mappings](https://www.benthos.dev/docs/guides/bloblang/about) the following build metadata as input:
| Parameter | Type | Description | Always Available |
| :--- | :---: | :--- | :---: |
| build_created_by | `string` | the username that created the build, only available when the resource is configured to [expose_build_created_by](https://concourse-ci.org/resources.html#schema.resource.expose_build_created_by) | |
| build_id | `string` | the unique build identifier | ✓ |
| build_instance_vars | `string` | instance vars of the instanced pipeline that the build's job lives in, serialized as JSON | |
| build_job | `string` | the name of the job | ✓ |
| build_name | `string` | the build id in the context of the pipeline | ✓ |
| build_pipeline | `string` | the name of the pipeline | ✓ |
| build_team | `string` | the name of the team | ✓ |
| build_url | `string` | the fully qualified build url | ✓ |

## License
Licensed under the [MIT License](LICENSE.md)  
Copyright (c) 2022 Chris Ludden