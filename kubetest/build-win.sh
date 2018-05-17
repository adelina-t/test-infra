#!/bin/bash
# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o pipefail

KUBEPATH=$GOPATH/src/k8s.io/kubernetes
DIST_DIR=$HOME/k
ZIP_PATH=$1

mkdir -p $DIST_DIR

$KUBEPATH/build/run.sh make WHAT=cmd/kubelet KUBE_BUILD_PLATFORMS=linux/amd64

build_kubectl() {
       echo "building kubectl.exe..."
       $KUBEPATH/build/run.sh make WHAT=cmd/kubectl KUBE_BUILD_PLATFORMS=windows/amd64
       cp ${GOPATH}/src/k8s.io/kubernetes/_output/dockerized/bin/windows/amd64/kubectl.exe ${DIST_DIR}
}

build_kubelet() {
	echo "building kubelet.exe..."
	$KUBEPATH/build/run.sh make WHAT=cmd/kubelet KUBE_BUILD_PLATFORMS=windows/amd64
	cp ${GOPATH}/src/k8s.io/kubernetes/_output/dockerized/bin/windows/amd64/kubelet.exe ${DIST_DIR}
}

build_kubeproxy() {
	echo "building kube-proxy.exe..."
	$KUBEPATH/build/run.sh make WHAT=cmd/kube-proxy KUBE_BUILD_PLATFORMS=windows/amd64
	cp ${GOPATH}/src/k8s.io/kubernetes/_output/dockerized/bin/windows/amd64/kube-proxy.exe ${DIST_DIR}
}

download_nssm() {
	NSSM_VERSION=2.24
	NSSM_URL=https://nssm.cc/release/nssm-${NSSM_VERSION}.zip
	echo "downloading nssm ..."
	curl ${NSSM_URL} -o /tmp/nssm-${NSSM_VERSION}.zip
	unzip -q -d /tmp /tmp/nssm-${NSSM_VERSION}.zip
	cp /tmp/nssm-${NSSM_VERSION}/win64/nssm.exe ${DIST_DIR}
	chmod 775 ${DIST_DIR}/nssm.exe
	rm -rf /tmp/nssm-${NSSM_VERSION}*
}

download_wincni() {
	mkdir -p ${DIST_DIR}/cni/config
	WINSDN_URL=https://github.com/Microsoft/SDN/raw/master/Kubernetes/windows/
	WINCNI_EXE=cni/wincni.exe
	HNS_PSM1=hns.psm1
	curl -L ${WINSDN_URL}${WINCNI_EXE} -o ${DIST_DIR}/${WINCNI_EXE}
	curl -L ${WINSDN_URL}${HNS_PSM1} -o ${DIST_DIR}/${HNS_PSM1}
}

download_pause_dockerfile() {
	wget https://raw.githubusercontent.com/Azure/acs-engine/master/windows/Dockerfile -P ${DIST_DIR}
}

create_zip() {
	cd ${DIST_DIR}/..
	zip -r $ZIP_PATH k/*
	cd -
}

build_kubelet
build_kubeproxy
build_kubectl
download_nssm
download_wincni
download_pause_dockerfile
create_zip

