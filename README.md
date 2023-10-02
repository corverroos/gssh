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
echo "export GSSH_USER=bar" >> ~/.bashrc
```

## Usage

```shell
# SSH by selecting one of all VMs:
gssh

# SSH by selecting one of any VMs that match regex 'foo' (name contains 'foo')
gssh -f foo

# SSH to a specific VM named 'foo-bar':
gssh -h foo-bar
gssh -f '^foo-bar$'

# SSH to previously selected VM:
gssh -p

# SSH using gcloud default user:
gssh -u=""

# SSH to previously selected VM and execute the command 'ls -la':
gssh -p ls -la

# SSH to VM named 'foo-bar' and execute the commands 'ls -la' and 'pwd' and 'printenv':
gssh -h foo-bar -- \
  ls -ls && \
  pwd && \
  printenv
``````
