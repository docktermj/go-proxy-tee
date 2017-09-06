# go-proxy-tee

Build `go-proxy-tee-M.m.P-I.x86_64.rpm`
and   `go-proxy-tee_M.m.P-I_amd64.deb`
where "M.m.P-I" is Major.minor.Patch-Iteration.

## Usage

A proxy the acts like Linux's `tee` command, but for network traffic.

### Configuration

Copy 
[go-proxy-tee.json.sample](go-proxy-tee.json-sample) 
to `go-proxy-tee.json` 
in one of the following places:
- `./go-proxy-tee.json`  for "directory-specific" invocation
- `$HOME/.go-proxy-tee/go-proxy-tee.json` for "user-specific" invocation
- `/etc/go-proxy-tee/go-proxy-tee.json` for "system-specific" invocation
Note: Can be placed in another directory and then use the `--configPath` command-line option.

Modify `go-proxy-tee.json` key/values:

- **debug:** Turn on/off debugging statements.
   - Values: true / false
   - Also available via the `--debug` command-line option
- **format:** Specify output format for "tee" files.
   - Values: "string", "hex", "binaryxml".
   - Also available via the `--format` command-line option
- **inbound:** Communication from client to `go-proxy-tee` 
   - **network:** Type of network. Values: "tcp", "unix"
   - **address:** Address for network-type.
- **outbound:** Communication from `go-proxy-tee` to primary server
   - **network:** Type of network. Values: "tcp", "unix"
   - **address:** Address for network-type.
   - **output:** File to send captured network traffic
   - Responses from the primary server will be transmitted to the client.
- **tee:** List of communications from `go-proxy-tee to additional servers 
   - **{tee-name}:** - a name of your choosing
      - **network:** Type of network. Values: "tcp", "unix"
      - **address:** Address for network-type.
      - **output:** File to send captured network traffic
   - Responses from these servers will not be transmitted to the client.

### Invocation

```console
go-proxy-tee net
```

## Development

### Dependencies

#### Set environment variables

```console
export GOPATH="${HOME}/go"
export PATH="${PATH}:${GOPATH}/bin:/usr/local/go/bin"
export PROJECT_DIR=${GOPATH}/src/github.com/docktermj
export REPOSITORY_DIR="${PROJECT_DIR}/go-proxy-tee"
```

#### Download project

```console
mkdir -p ${PROJECT_DIR}
cd ${PROJECT_DIR}
git clone git@github.com:docktermj/go-proxy-tee.git
```

#### Download dependencies

```console
cd ${REPOSITORY_DIR}
make dependencies
```

### Build

#### Local build

```console
cd ${REPOSITORY_DIR}
make build-local
```

The results will be in the `${GOPATH}/bin` directory.

#### Docker build

```console
cd ${REPOSITORY_DIR}
make build
```

The results will be in the `.../target` directory.

### Test

```console
cd ${REPOSITORY_DIR}
make test-local
```

### Install

#### RPM-based

Example distributions: openSUSE, Fedora, CentOS, Mandrake

##### RPM Install

Example:

```console
sudo rpm -ivh go-proxy-tee-M.m.P-I.x86_64.rpm
```

##### RPM Update

Example: 

```console
sudo rpm -Uvh go-proxy-tee-M.m.P-I.x86_64.rpm
```

#### Debian

Example distributions: Ubuntu

##### Debian Install / Update

Example:

```console
sudo dpkg -i go-proxy-tee_M.m.P-I_amd64.deb
```

### Cleanup

```console
make clean
```
