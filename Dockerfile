FROM centos:7

ENV REFRESHED_AT 2017-11-28

ARG PROGRAM_NAME="unknown"
ARG BUILD_VERSION=0.0.0
ARG BUILD_ITERATION=0

# Install dependencies.
RUN yum -y install \
    curl \
    gcc \
    git \
    make \
    rpm-build \
    ruby-devel \
    rubygems \
    tar \
    which

# --- Install Go --------------------------------------------------------------

ENV GO_VERSION=1.8.3
ENV GO_TGZ=go${GO_VERSION}.linux-amd64.tar.gz
ENV GO_URL=https://storage.googleapis.com/golang/${GO_TGZ}

# Install "go".
RUN curl -o ${GO_TGZ} ${GO_URL} && \
    tar -C /usr/local/ -xzf ${GO_TGZ}

# --- Install Effing Package Manager (FPM) ------------------------------------

RUN gem install --no-ri --no-rdoc fpm

# --- Compile go program ------------------------------------------------------

ENV HOME="/root"
ENV GOPATH="${HOME}/gocode"
ENV PATH="${PATH}:/usr/local/go/bin:${GOPATH}/bin"
ENV GO_PACKAGE="github.com/docktermj/${PROGRAM_NAME}"

# Install dependencies.
RUN go get \
    github.com/docopt/docopt-go \
    github.com/spf13/viper \
    github.com/BixData/binaryxml \
    github.com/jnewmoyer/xmlpath \
    github.com/go-xmlfmt/xmlfmt

# Copy local files from the Git repository.
COPY . ${GOPATH}/src/${GO_PACKAGE}

# Build go program.
RUN go install \
    -ldflags "-X main.programName=${PROGRAM_NAME} -X main.buildVersion=${BUILD_VERSION} -X main.buildIteration=${BUILD_ITERATION}" \
    ${GO_PACKAGE}

# Copy binary to output.
RUN mkdir -p /output/bin && \
    cp /root/gocode/bin/${PROGRAM_NAME} /output/bin

# --- Test go program ---------------------------------------------------------

# Run unit tests
RUN go get github.com/jstemmer/go-junit-report && \
    mkdir -p /output/go-junit-report && \
    go test -v ${GO_PACKAGE}/... | go-junit-report > /output/go-junit-report/test-report.xml

# --- Package as RPM and DEB --------------------------------------------------

WORKDIR /output

# RPM package.
RUN fpm \
  --input-type dir \
  --output-type rpm \
  --name ${PROGRAM_NAME} \
  --version ${BUILD_VERSION} \
  --iteration ${BUILD_ITERATION} \
  /root/gocode/bin/=/usr/bin

# DEB package.
RUN fpm \
  --input-type dir \
  --output-type deb \
  --name ${PROGRAM_NAME} \
  --version ${BUILD_VERSION} \
  --iteration ${BUILD_ITERATION} \
  /root/gocode/bin/=/usr/bin

# --- Epilog ------------------------------------------------------------------

RUN yum clean all

CMD ["/bin/bash"]
