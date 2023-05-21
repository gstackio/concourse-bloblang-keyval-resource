# concourse-keyval-resource
a [Concourse](https://concourse-ci.org/resources.html) resource for:
- passing arbitrary data between steps and/or jobs
- curating dynamic filesystem content

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
  - name: first
    plan:
      - put: keyval
        params:
          mapping: |
            root = this.map_each_key(k -> k.replace_all("build_", ""))

  - name: second
    plan:
      - get: keyval
        trigger: true
        passed: [first]
        params:
          files:
            test.md: |
              root = """
              # Build Summary
              | ID | Team | Pipeline | Job | Name |
              | :---: | :--- | :--- | :--- | :--- |
              | [%s](%s) | %s | %s | %s | %s |
              """.format(id, url, team, pipeline, job, name)
            test.json: |
              uid = ksuid()
              id = id
              url = url

      - task: print
        config:
          platform: linux
          image_resource:
            type: registry-image
            source:
              repository: busybox
          inputs:
            - name: keyval
          run:
            path: /bin/sh
            args:
              - -c
              - |
                ls -lh ./keyval
                echo "keyval/version.json:"
                cat keyval/version.json
                echo "keyval/metadata.json:"
                cat keyval/metadata.json
                echo "keyval/test.md:"
                cat keyval/test.md
                echo "keyval/test.json:"
                cat keyval/test.json
```

## Configuration

**Parameters:**
| Parameter | Type | Description | Required |
| :--- | :---: | :--- | :---: |
| archive | [*archive.Archive](https://pkg.go.dev/github.com/cludden/concourse-go-sdk@v0.3.1/pkg/archive#Config) | optional archive config that can be used to enable [resource version archiving](https://github.com/cludden/concourse-go-sdk#archiving) | |
| initial_mapping | `string` | a [Bloblang mapping](https://www.benthos.dev/docs/guides/bloblang/about) that can be used to customize the initial version returned by this resource. See [Initial Mapping](#initial-mapping) for more details. | ⚠ |

## Behavior

### `check`
Check returns the latest available version produced by a `put` step. If `get` is used prior to `put`, an initial version will be returned which can be customized via the `initial_mapping` resource parameter.

### `in`
Fetches arbitrary key value data from a prior put step and writes it the file system as a JSON document.

**Parameters:**
| Parameter | Type | Description | Required |
| :--- | :---: | :--- | :---: |
| files | `map[string]string` | a map of filenames to [Bloblang mappings](https://www.benthos.dev/docs/guides/bloblang/about), where the input document contains the [build metadata](https://concourse-ci.org/implementing-resource-types.html#resource-metadata) along with any key value data from the fetched version, and the output is the content of the file to write (note: unless the file extension is one of `.json`, `.yaml`, or `.yml`, the mapping output must be of type `string` or `[]byte`) | |

**Files:**
- `version.json` - the key value pairs serialized as a JSON document
- `metadata.json` - the [build metadata](https://concourse-ci.org/implementing-resource-types.html#resource-metadata) serialized as a JSON document
- `*` any file mappings defined in the `files` parameter

### `out`
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

## Initial Mapping
In most sitautions, like the example below, the initial use of this resource is with a `put` step, followed by a later `get` step, in which case this field is not required and checking should be disabled via `check_every: never` to conserve resources. 

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
    check_every: never
    expose_build_created_by: true

jobs:
  - name: first
    plan:
      - put: keyval
        params:
          mapping: |
            timestamp = now()
  
  - name: second
    plan:
      - get: keyval
        passed: [first]
        trigger: true
```

In others, this resource may be initially used with a `get` step (e.g. importing data from a previous build of the same job), in which case this field is required and `check_every` should be set to some value other than `never` in order to allow for Concourse to implicitly run a check when attempting to satisfy inputs for the initial build.

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
    check_every: 24h
    expose_build_created_by: true
    source:
      initial_mapping: |
        count = "0"

jobs:
  - name: first
    plan:
      - get: previous
        resource: keyval

      - load_var: data
        file: previous/version.json
        reveal: true

      - put: keyval
        params:
          mapping: |
            count = (((.:data.count)) + 1).string()
```

## License
Licensed under the [MIT License](LICENSE.md)  
Copyright (c) 2023 Chris Ludden