# gssh

Command `gssh` is a wrapper around `gcloud compute ssh` that allows for a non-default ssh-username
and autocompletes VM names. 

## Setup

```shell
# Setup gcloud:
gcloud auth login
gcloud config set project foo

# Install gssh:
go install github.com/corverroos/gssh

# If `which gssh` fails, then fix your environment: `export PATH=$PATH:$(go env GOPATH)/bin`. Or see https://go.dev/doc/gopath_code

# Setup ssh user via GSSH_USER env var:
echo "GSSH_USER=bar" >> ~/.bashrc
```

## Usage

```shell
# SSH by selecting one of all VMs:
gssh

# SSH by selecting one of any VMs that start with "foo":
gssh foo

# SSH to a specific VM:
gssh foo-bar
``````