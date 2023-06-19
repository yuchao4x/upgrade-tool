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

package labels

// This file contains constants for frequently used labels.

// BundleExtracted is indicates that a node has the bundle files extracted into the a directory.
const BundleExtracted = prefix + "/bundle-extracted"

// BundleLoaded is indicates that a node has the images loaded into the CRI-O storage.
const BundleLoaded = prefix + "/bundle-loaded"

// BundleCleaned is indicates that a node has been cleaned after the upgrade.
const BundleCleaned = prefix + "/bundle-cleaned"

// Job contains the name the job.
const Job = prefix + "/job"

// App contains the name of the application.
const App = prefix + "/app"

// prefix is the prefix for all the annotations.
const prefix = "upgrade-tool"
