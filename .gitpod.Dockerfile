FROM gitpod/workspace-full

USER root

# install go releaser
RUN cd /usr/bin && curl -L https://github.com/goreleaser/goreleaser/releases/download/v0.117.1/goreleaser_Linux_x86_64.tar.gz | tar xz
