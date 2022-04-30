# concourse-keyval-resource
a [Concourse] resource for passing arbitrary data between jobs

## Getting Started
```yaml
resource_types:
- name: keyval
  type: registry-image
  source:
    repository: cludden/concourse-kayval-resource

resources:
- name: keyval
  type: keyval
  icon: bucket

- name: repo
  type: get
  icon: github
  source:
    uri: https://github.com/cludden/concourse-keyval-resource.git

jobs:
- name: first
  plan:
  - get: repo
    trigger: true

  - put: keyval
    params:
      mapping: |
        id = ksuid()
        ref = file("repo/.git/ref").string()
        url = url

- name: second
  plan:
  - get: keyval
    trigger: true
    params:
      files:
        test.yml: |
          root = this.format_yaml()

  - load_var: version
    file: keyval/version.json

  - task: echo
    config:
      platform: linux
      image_resource:
        type: registry-image
        source: { repository: busybox }
      run:
        path: /bin.sh
        args: 
          - -c
          - |
            echo "id: ((.:version.id))"
            echo "ref: ((.:version.ref))"
            echo "url: ((.:version.url))"
            echo "version.json"
            cat keyval/version.json
            echo "test.yml"
            cat keyval/test.yml
```

## Behavior

### `check`
Not implemented

### `in`
Fetches arbitrary key value data from a prior put step and writes it the file system as a JSON document.

**Parameters:**
| Parameter | Type | Description | Required |
| :--- | :---: | :--- | :---: |
| files | `map[string]string` | a map of filenames to [Bloblang mappings](https://www.benthos.dev/docs/guides/bloblang/about), where the input document contains the build metadata along with any key value data from the fetched version, and the output is a `string` or `[]byte` that is the content of the file to write | |

**Files:**
- `version.json` - the key value pairs serialized as JSON
- `*` any file mappings defined in the `files` parameter

### `out`: publish arbitrary key value data
Publishes arbitrary key value data to be shared across jobs.

**Parameters:**
| Parameter | Type | Description | Required |
| :--- | :---: | :--- | :---: |
| mapping | `string` | a [Bloblang mappings](https://www.benthos.dev/docs/guides/bloblang/about), where the input document contains the [build metadata](https://concourse-ci.org/implementing-resource-types.html#resource-metadata), and the output is a `map[string]string` | ✓ |

## Build Metadata
Each [Bloblang mappings](https://www.benthos.dev/docs/guides/bloblang/about) the following build metadata as input:
| Parameter | Type | Description | Always Available |
| :--- | :---: | :--- | :---: |
| build_id | `string` | the unique build identifier | ✓ |
| build_name | `string` | the build id in the context of the pipeline | ✓ |
| build_job | `string` | the name of the job | ✓ |
| build_pipeline | `string` | the name of the pipeline | ✓ |
| build_team | `string` | the name of the team | ✓ |
| build_instance_vars | `string` | instance vars of the instanced pipeline that the build's job lives in, serialized as JSON | |
| build_created_by | `string` | the username that created the build, only available when the resource is configured to [expose_build_created_by](https://concourse-ci.org/resources.html#schema.resource.expose_build_created_by) | |
| build_url | `string` | the fully qualified build url |

## License
Licensed under the [MIT License](LICENSE.md)  
Copyright (c) 2022 Chris Ludden