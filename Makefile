#
# Copyright (c) 2023 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License"); you may not
# use this file except in compliance with the License. You may obtain a copy
# of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
# WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and limitations under
# the License.
#

# Full image reference:
image:=quay.io/jhernand/upgrade-tool:latest

.PHONY: build
build:
	go build

.PHONY: image
image: build
	podman build -t "$(image)" .

.PHONY: push
push: image
	podman push "$(image)"

.PHONY: deploy
deploy: push
	oc create -f controller.yaml

.PHONY: undeploy
undeploy:
	oc delete -f controller.yaml
