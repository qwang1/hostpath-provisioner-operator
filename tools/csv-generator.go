/*
Copyright 2019 The hostpath provisioner operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"encoding/json"
	"flag"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"github.com/blang/semver"
	csvv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/version"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubevirt.io/hostpath-provisioner-operator/pkg/controller/hostpathprovisioner"
	"kubevirt.io/hostpath-provisioner-operator/tools/helper"
	"kubevirt.io/hostpath-provisioner-operator/tools/util"
)

var (
	csvVersion         = flag.String("csv-version", "", "")
	replacesCsvVersion = flag.String("replaces-csv-version", "", "")
	namespace          = flag.String("namespace", "", "")
	pullPolicy         = flag.String("pull-policy", "", "")

	logoBase64 = flag.String("logo-base64", "", "")
	verbosity  = flag.String("verbosity", "1", "")

	operatorImage    = flag.String("operator-image-name", hostpathprovisioner.OperatorImageDefault, "optional")
	provisionerImage = flag.String("provisioner-image-name", hostpathprovisioner.ProvisionerImageDefault, "optional")
	dumpCRDs         = flag.Bool("dump-crds", false, "optional - dumps operator related crd manifests to stdout")
)

func main() {
	flag.Parse()

	data := NewClusterServiceVersionData{
		CsvVersion:         *csvVersion,
		ReplacesCsvVersion: *replacesCsvVersion,
		Namespace:          *namespace,
		ImagePullPolicy:    *pullPolicy,
		IconBase64:         *logoBase64,
		Verbosity:          *verbosity,

		OperatorImage:    *operatorImage,
		ProvisionerImage: *provisionerImage,
	}

	csv, err := createClusterServiceVersion(&data)
	if err != nil {
		panic(err)
	}
	util.MarshallObject(csv, os.Stdout)

	if *dumpCRDs {
		cidCrd := helper.CreateCRDDef()
		util.MarshallObject(cidCrd, os.Stdout)
	}
}

//NewClusterServiceVersionData - Data arguments used to create hostpath provisioner's CSV manifest
type NewClusterServiceVersionData struct {
	CsvVersion         string
	ReplacesCsvVersion string
	Namespace          string
	ImagePullPolicy    string
	IconBase64         string
	Verbosity          string

	DockerPrefix string
	DockerTag    string

	OperatorImage    string
	ProvisionerImage string
}

type csvPermissions struct {
	ServiceAccountName string              `json:"serviceAccountName"`
	Rules              []rbacv1.PolicyRule `json:"rules"`
}
type csvDeployments struct {
	Name string                `json:"name"`
	Spec appsv1.DeploymentSpec `json:"spec,omitempty"`
}

type csvStrategySpec struct {
	Permissions        []csvPermissions `json:"permissions,omitempty"`
	ClusterPermissions []csvPermissions `json:"clusterPermissions"`
	Deployments        []csvDeployments `json:"deployments"`
}

func createOperatorDeployment(repo, namespace, deployClusterResources, operatorImage, provisionerImage, tag, verbosity, pullPolicy string) *appsv1.Deployment {
	deployment := helper.CreateOperatorDeployment("hostpath-provisioner-operator", namespace, "name", "hostpath-provisioner-operator", hostpathprovisioner.OperatorServiceAccountName, int32(1))
	container := helper.CreateOperatorContainer("hostpath-provisioner-operator", operatorImage, verbosity, corev1.PullPolicy(pullPolicy))
	container.Env = *helper.CreateOperatorEnvVar(repo, deployClusterResources, operatorImage, provisionerImage, pullPolicy)
	deployment.Spec.Template.Spec.Containers = []corev1.Container{container}
	return deployment
}

func createClusterServiceVersion(data *NewClusterServiceVersionData) (*csvv1.ClusterServiceVersion, error) {

	description := `
Hostpath provisioner is a local storage provisioner that uses kubernetes hostpath support to create directories on the host that map to a PV. These PVs are dynamically created when a new PVC is requested.
`

	deployment := createOperatorDeployment(
		data.DockerPrefix,
		data.Namespace,
		"true",
		data.OperatorImage,
		data.ProvisionerImage,
		data.DockerTag,
		data.Verbosity,
		data.ImagePullPolicy)

	clusterRules := getOperatorClusterRules()
	rules := getOperatorRules()

	strategySpec := csvStrategySpec{
		ClusterPermissions: []csvPermissions{
			{
				ServiceAccountName: hostpathprovisioner.OperatorServiceAccountName,
				Rules:              *clusterRules,
			},
		},
		Permissions: []csvPermissions{
			{
				ServiceAccountName: hostpathprovisioner.OperatorServiceAccountName,
				Rules:              *rules,
			},
		},
		Deployments: []csvDeployments{
			{
				Name: "hostpath-provisioner-operator",
				Spec: deployment.Spec,
			},
		},
	}

	strategySpecJSONBytes, err := json.Marshal(strategySpec)
	if err != nil {
		return nil, err
	}

	csvVersion, err := semver.New(data.CsvVersion)
	if err != nil {
		return nil, err
	}

	return &csvv1.ClusterServiceVersion{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterServiceVersion",
			APIVersion: "operators.coreos.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hostpathprovisioneroperator." + data.CsvVersion,
			Namespace: data.Namespace,
			Annotations: map[string]string{

				"capabilities": "Full Lifecycle",
				"categories":   "Storage",
				"alm-examples": `
      [
        {
		  "apiVersion": "hostpathprovisioner.kubevirt.io/v1alpha1",
		  "kind": "HostPathProvisioner",
		  "metadata": {
			"name": "hostpath-provisioner"
		  },
		  "spec": {
			"imagePullPolicy":"IfNotPresent",
			"pathConfig": {
			  "path": "/var/hpvolumes",
			  "useNamingPrefix": "false"
			}
          }
        }
      ]`,
				"description": "Creates and maintains hostpath provisioner deployments",
			},
		},

		Spec: csvv1.ClusterServiceVersionSpec{
			DisplayName: "Hostpath Provisioner",
			Description: description,
			Keywords:    []string{"Hostpath Provisioner", "Storage"},
			Version:     version.OperatorVersion{Version: *csvVersion},
			Maturity:    "alpha",
			Replaces:    data.ReplacesCsvVersion,
			Maintainers: []csvv1.Maintainer{{
				Name:  "KubeVirt project",
				Email: "kubevirt-dev@googlegroups.com",
			}},
			Provider: csvv1.AppLink{
				Name: "KubeVirt/Hostpath-provisioner project",
			},
			Links: []csvv1.AppLink{
				{
					Name: "Hostpath Provisioner",
					URL:  "https://github.com/kubevirt/hostpath-provisioner/blob/master/README.md",
				},
				{
					Name: "Source Code",
					URL:  "https://github.com/kubevirt/hostpath-provisioner",
				},
			},
			Icon: []csvv1.Icon{{
				Data:      data.IconBase64,
				MediaType: "image/png",
			}},
			Labels: map[string]string{
				"alm-owner-hostpath-provisioner": "hostpath-provisioner-operator",
				"operated-by":                    "hostpath-provisioner-operator",
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"alm-owner-hostpath-provisioner": "hostpath-provisioner-operator",
					"operated-by":                    "hostpath-provisioner-operator",
				},
			},
			InstallModes: []csvv1.InstallMode{
				{
					Type:      csvv1.InstallModeTypeOwnNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeSingleNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeMultiNamespace,
					Supported: true,
				},
				{
					Type:      csvv1.InstallModeTypeAllNamespaces,
					Supported: true,
				},
			},
			InstallStrategy: csvv1.NamedInstallStrategy{
				StrategyName:    "deployment",
				StrategySpecRaw: json.RawMessage(strategySpecJSONBytes),
			},
			CustomResourceDefinitions: csvv1.CustomResourceDefinitions{

				Owned: []csvv1.CRDDescription{
					{
						Name:        "hostpathprovisioners.hostpathprovisioner.kubevirt.io",
						Version:     "v1alpha1",
						Kind:        "HostPathProvisioner",
						DisplayName: "HostPathProvisioner deployment",
						Description: "Represents a HostPathProvisioner deployment",
						SpecDescriptors: []csvv1.SpecDescriptor{

							{
								Description:  "The ImageRegistry to use for the HostPathProvisioner components.",
								DisplayName:  "ImageRegistry",
								Path:         "imageRegistry",
								XDescriptors: []string{"urn:alm:descriptor:text"},
							},
							{
								Description:  "The ImageTag to use for the HostPathProvisioner components.",
								DisplayName:  "ImageTag",
								Path:         "imageTag",
								XDescriptors: []string{"urn:alm:descriptor:text"},
							},
							{
								Description:  "The ImagePullPolicy to use for the HostPathProvisioner components.",
								DisplayName:  "ImagePullPolicy",
								Path:         "imagePullPolicy",
								XDescriptors: []string{"urn:alm:descriptor:io.kubernetes:imagePullPolicy"},
							},
							{
								Description:  "The PathConfig object allows you to specify where and how you want the directories created that will back the PVs.",
								DisplayName:  "PathConfig",
								Path:         "pathConfig",
								XDescriptors: []string{"urn:alm:descriptor:text"},
							},
						},
						StatusDescriptors: []csvv1.StatusDescriptor{
							{
								Description:  "Explanation for the current status of the HostPathProvisioner deployment.",
								DisplayName:  "Conditions",
								Path:         "conditions",
								XDescriptors: []string{"urn:alm:descriptor:io.kubernetes.conditions"},
							},
							{
								Description:  "The observed version of the HostPathProvisioner deployment.",
								DisplayName:  "Observed HostPathProvisioner Version",
								Path:         "observedVersion",
								XDescriptors: []string{"urn:alm:descriptor:text"},
							},
							{
								Description:  "The targeted version of the HostPathProvisioner deployment.",
								DisplayName:  "Target HostPathProvisioner Version",
								Path:         "targetVersion",
								XDescriptors: []string{"urn:alm:descriptor:text"},
							},
							{
								Description:  "The version of the HostPathProvisioner Operator",
								DisplayName:  "HostPathProvisioner Operator Version",
								Path:         "operatorVersion",
								XDescriptors: []string{"urn:alm:descriptor:text"},
							},
						},
					},
				},
			},
		},
	}, nil
}

func getOperatorRules() *[]rbacv1.PolicyRule {
	return &[]rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"daemonsets",
			},
			Verbs: []string{
				"list",
				"get",
				"watch",
				"create",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"daemonsets",
			},
			ResourceNames: []string{
				"hostpath-provisioner",
			},
			Verbs: []string{
				"delete",
				"update",
			},
		},
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"pods",
			},
			Verbs: []string{
				"get",
			},
		},
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"configmaps",
			},
			Verbs: []string{
				"get",
				"create",
			},
		},
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"serviceaccounts",
			},
			ResourceNames: []string{
				"hostpath-provisioner-admin",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"serviceaccounts",
			},
			Verbs: []string{
				"list",
				"get",
				"watch",
				"create",
			},
		},
	}
}

func getOperatorClusterRules() *[]rbacv1.PolicyRule {
	return &[]rbacv1.PolicyRule{
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"persistentvolumes",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"persistentvolumeclaims",
			},
			Verbs: []string{
				"get",
				"list",
				"watch",
				"create",
				"update",
			},
		},
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"events",
			},
			Verbs: []string{
				"list",
				"watch",
				"create",
				"patch",
				"update",
			},
		},
		{
			APIGroups: []string{
				"rbac.authorization.k8s.io",
			},
			Resources: []string{
				"clusterrolebindings",
			},
			ResourceNames: []string{
				"hostpath-provisioner",
			},
			Verbs: []string{
				"update",
				"delete",
			},
		},
		{
			APIGroups: []string{
				"rbac.authorization.k8s.io",
			},
			Resources: []string{
				"clusterrolebindings",
			},
			Verbs: []string{
				"list",
				"get",
				"watch",
				"create", //Need watch and create here or it cannot create the specific hostpath-provisioner cluster roles, cannot put watch and create on the specific resourceName
			},
		},
		{
			APIGroups: []string{
				"rbac.authorization.k8s.io",
			},
			Resources: []string{
				"clusterroles",
			},
			ResourceNames: []string{
				"hostpath-provisioner",
			},
			Verbs: []string{
				"update",
				"delete",
			},
		},
		{
			APIGroups: []string{
				"rbac.authorization.k8s.io",
			},
			Resources: []string{
				"clusterroles",
			},
			Verbs: []string{
				"list",
				"get",
				"watch",
				"create",
			},
		},
		{
			APIGroups: []string{
				"apps",
			},
			Resources: []string{
				"deployments/finalizers",
			},
			ResourceNames: []string{
				"hostpath-provisioner-operator",
			},
			Verbs: []string{
				"update",
			},
		},
		{
			APIGroups: []string{
				"hostpathprovisioner.kubevirt.io",
			},
			Resources: []string{
				"*",
			},
			Verbs: []string{
				"*",
			},
		},
		{
			APIGroups: []string{
				"security.openshift.io",
			},
			Resources: []string{
				"securitycontextconstraints",
			},
			Verbs: []string{
				"list",
				"get",
				"watch",
				"create",
			},
		},
		{
			APIGroups: []string{
				"security.openshift.io",
			},
			Resources: []string{
				"securitycontextconstraints",
			},
			ResourceNames: []string{
				"hostpath-provisioner",
			},
			Verbs: []string{
				"delete",
				"update",
			},
		},
		{
			APIGroups: []string{
				"storage.k8s.io",
			},
			Resources: []string{
				"storageclasses",
			},
			Verbs: []string{
				"get",
				"list",
			},
		},
		{
			APIGroups: []string{
				"storage.k8s.io",
			},
			Resources: []string{
				"storageclasses",
			},
			ResourceNames: []string{
				"hostpath-provisioner",
			},
			Verbs: []string{
				"watch",
			},
		},
		{
			APIGroups: []string{
				"",
			},
			Resources: []string{
				"nodes",
			},
			Verbs: []string{
				"get",
			},
		},
	}
}
