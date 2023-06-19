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

package annotations

// This file contains constants for frequently used annotations.

// BundleFile is the annotation that contains the name of the bundle file.
const BundleFile = prefix + "/bundle-file"

// FundleMetadata contains the metadata of the bundle, except the list of images.
const BundleMetadata = prefix + "/bundle-metadata"

// Progress contains information about the progress of the upgrade.
const Progress = prefix + "/progress"

// prefix is the prefix for all the annotations.
const prefix = "upgrade-tool"
