/*
Copyright 2023 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in
compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is
distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing permissions and limitations under the
License.
*/

package internal

// Metadata describes an upgrade package. This will be serialized to JSON and added to the tar
// archive as the first item, named `metadata.json`.
type Metadata struct {
	Version string   `json:"version,omitempty"`
	Arch    string   `json:"arch,omitempty"`
	Release string   `json:"release,omitempty"`
	Images  []string `json:"images,omitempty"`
}
