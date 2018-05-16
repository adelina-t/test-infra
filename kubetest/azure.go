/*
Copyright 2018 The Kubernetes Authors.

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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/pelletier/go-toml"

	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/satori/go.uuid"
        "github.com/Azure/azure-storage-blob-go/2016-05-31/azblob"
)

var (
	// azure specific flags
	acsResourceName      = flag.String("acsengine-resource-name", "", "Azure Resource Name")
	acsResourceGroupName = flag.String("acsengine-resourcegroup-name", "", "Azure Resource Group Name")
	acsLocation          = flag.String("acsengine-location", "westus2", "Azure ACS location")
	acsMasterVmSize      = flag.String("acsengine-mastervmsize", "Standard_D2s_v3", "Azure Master VM size")
	acsAgentVmSize       = flag.String("acsengine-agentvmsize", "Standard_D2s_v3", "Azure Agent VM size")
	acsAdminUsername     = flag.String("acsengine-admin-username", "", "Admin username")
	acsAdminPassword     = flag.String("acsengine-admin-password", "", "Admin password")
	acsAgentPoolCount    = flag.Int("acsengine-agentpoolcount", 2, "Azure Agent Pool Count")
	acsTemplatePath      = flag.String("acsengine-template", "", "Azure Template Name")
	acsDnsPrefix         = flag.String("acsengine-dnsprefix", "", "Azure K8s Master DNS Prefix")
	acsEngineURL         = flag.String("acsengine-download-url", "", "Download URL for ACS engine")
	acsEngineMD5         = flag.String("acsengine-md5-sum", "", "Checksum for acs engine download")
	acsSSHPublicKeyPath  = flag.String("acsengine-public-key", "", "Path to SSH Public Key")
	acsWinBinariesURL    = flag.String("acsengine-win-binaries-url", "", "Path to get the zip file containing kubelet and kubeproxy binaries for Windows")
	acsHyperKubeURL      = flag.String("acsengine-hypercube-url", "", "Path to get the kyberkube image for the deployment")
	acsCredentialsFile   = flag.String("acsengine-creds", "", "Path to credential file for Azure")
)

type Creds struct {
	ClientID       string
	ClientSecret   string
	TennantID      string
	SubscriptionId string
        StorageAccountName string
        StorageAccountKey string
}

type Config struct {
	Creds Creds
}

type Cluster struct {
	ctx                 context.Context
	credentials         *Creds
	location            string
	resourceGroup       string
	name                string
	apiModelPath        string
	dnsPrefix           string
	templateJSON        map[string]interface{}
	parametersJSON      map[string]interface{}
	outputDir           string
	sshPublicKey        string
	adminUsername       string
	adminPassword       string
	masterVMSize        string
	agentVMSize         string
	acsEngineBinaryPath string
	azureClient         *AzureClient
}

func (c *Cluster) getAzCredentials() error {
	content, err := ioutil.ReadFile(*acsCredentialsFile)
	if err != nil {
		return fmt.Errorf("Error reading credentials file %v %v", *acsCredentialsFile, err)
	}
	config := Config{}
	err = toml.Unmarshal(content, &config)
	c.credentials = &config.Creds
	if err != nil {
		return fmt.Errorf("Error parsing credentials file %v %v", *acsCredentialsFile, err)
	}
	return nil
}

func checkParams() error {
	if *acsCredentialsFile == "" {
		return fmt.Errorf("No credentials file path specified")
	}
	if *acsResourceName == "" {
		*acsResourceName = "kubetest-" + uuid.NewV1().String()
	}
	if *acsResourceGroupName == "" {
		*acsResourceGroupName = *acsResourceName + "-rg"
	}
	if *acsDnsPrefix == "" {
		*acsDnsPrefix = *acsResourceName
	}
	if *acsSSHPublicKeyPath == "" {
		*acsSSHPublicKeyPath = os.Getenv("HOME") + "/.ssh/id_rsa.pub"
	}
	if *acsAdminUsername == "" {
		return fmt.Errorf("Error parsing flags. No admin username specified")
	}
	if *acsAdminPassword == "" {
		return fmt.Errorf("Error parting flags. No admin password specified.")
	}
	return nil
}

func newAzure() (*Cluster, error) {
	if err := checkParams(); err != nil {
		return nil, fmt.Errorf("Error creating Azure K8S cluster: %v", err)
	}

	tempdir, _ := ioutil.TempDir(os.Getenv("HOME"), "acs")
	sshKey, err := ioutil.ReadFile(*acsSSHPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("Error reading SSH Key %v %v", *acsSSHPublicKeyPath, err)
	}
	c := Cluster{
		ctx:           context.Background(),
		apiModelPath:  *acsTemplatePath,
		name:          *acsResourceName,
		dnsPrefix:     *acsDnsPrefix,
		location:      *acsLocation,
		resourceGroup: *acsResourceGroupName,
		outputDir:     tempdir,
		sshPublicKey:  fmt.Sprintf("%s", sshKey),
		credentials:   &Creds{},
	}
        c.getAzCredentials()
        err = c.getARMClient(c.ctx)
	if err != nil {
		return nil, fmt.Errorf("Failed to generate ARM client: %v", err)
	}

	return &c, nil
}

func (c *Cluster) generateTemplate() error {
	v := &AcsEngineApiModel{
		ApiVersion: "vlabs",
		Location:   c.location,
		Name:       c.name,
		Tags: map[string]string{
			"date": time.Now().String(),
		},
		Properties: &Properties{
			OrchestratorProfile: &OrchestratorProfile{
				OrchestratorType: "Kubernetes",
			},
			MasterProfile: &MasterProfile{
				Count:     1,
				DNSPrefix: c.dnsPrefix,
				VMSize:    *acsMasterVmSize,
			},
			AgentPoolProfiles: []*AgentPoolProfile{
				{
					Name:                "agentpool0",
					VMSize:              *acsAgentVmSize,
					Count:               *acsAgentPoolCount,
					OSType:              "Windows",
					AvailabilityProfile: "AvailabilitySet",
				},
			},
			LinuxProfile: &LinuxProfile{
				AdminUsername: *acsAdminUsername,
				SSHKeys: &SSH{
					PublicKeys: []PublicKey{{
						KeyData: c.sshPublicKey,
					},
					},
				},
			},
			WindowsProfile: &WindowsProfile{
				AdminUsername: *acsAdminUsername,
				AdminPassword: *acsAdminPassword,
			},
			ServicePrincipalProfile: &ServicePrincipalProfile{
				ClientID: c.credentials.ClientID,
				Secret:   c.credentials.ClientSecret,
			},
		},
	}
	if *acsHyperKubeURL != "" {
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomHyperkubeImage = *acsHyperKubeURL
	}
	if *acsWinBinariesURL != "" {
		v.Properties.OrchestratorProfile.KubernetesConfig.CustomWindowsPackageURL = *acsWinBinariesURL
	}
	apiModel, _ := json.Marshal(v)
	c.apiModelPath = path.Join(c.outputDir, "kubernetes.json")
	err := ioutil.WriteFile(c.apiModelPath, apiModel, 0644)
	if err != nil {
		return fmt.Errorf("Cannot write to file: %v", err)
	}
	return nil
}

func (c *Cluster) getAcsEngine(retry int) error {
	downloadPath := path.Join(os.Getenv("HOME"), "acs-engine.tar.gz")
	f, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := 0; i < retry; i++ {
		log.Printf("downloading %v from %v.", downloadPath, *acsEngineURL)
		if err := httpRead(*acsEngineURL, f); err == nil {
			break
		}
		err = fmt.Errorf("url=%s failed get %v: %v.", *acsEngineURL, downloadPath, err)
		if i == retry-1 {
			return err
		}
		log.Println(err)
		sleep(time.Duration(i) * time.Second)
	}

	f.Close()
	if *acsEngineMD5 != "" {
		o, err := control.Output(exec.Command("md5sum", f.Name()))
		if err != nil {
			return err
		}
		if strings.Split(string(o), " ")[0] != *acsEngineMD5 {
			return fmt.Errorf("Wrong md5 sum for acs-engine")
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get current directory: %v", err)
	}
	log.Printf("Extracting tar file %v into directory %v", f.Name(), cwd)

	if err = control.FinishRunning(exec.Command("tar", "-xzf", f.Name(), "--strip", "1")); err != nil {
		return err
	}
	c.acsEngineBinaryPath = path.Join(cwd, "acs-engine")
	return nil

}

func (c Cluster) generateARMTemplates() error {
	if err := control.FinishRunning(exec.Command(c.acsEngineBinaryPath, "generate", c.apiModelPath, "--output-directory", c.outputDir)); err != nil {
		return fmt.Errorf("Failed to generate ARM templates: %v", err)
	}
	return nil
}

func (c *Cluster) loadARMTemplates() error {
	var err error
	template, err := ioutil.ReadFile(path.Join(c.outputDir, "azuredeploy.json"))
	if err != nil {
		return fmt.Errorf("Error reading ARM template file: %v", err)
	}
	c.templateJSON = make(map[string]interface{})
	err = json.Unmarshal(template, &c.templateJSON)
	if err != nil {
		return fmt.Errorf("Error unmarshall template %v", err.Error())
	}
	parameters, err := ioutil.ReadFile(path.Join(c.outputDir, "azuredeploy.parameters.json"))
	if err != nil {
		return fmt.Errorf("Error reading ARM parameters file: %v", err)
	}
	c.parametersJSON = make(map[string]interface{})
	err = json.Unmarshal(parameters, &c.parametersJSON)
	if err != nil {
		return fmt.Errorf("Error unmarshall parameters %v", err.Error())
	}
	c.parametersJSON = c.parametersJSON["parameters"].(map[string]interface{})

	return nil
}

func (c *Cluster) getARMClient(ctx context.Context) error {
	env, err := azure.EnvironmentFromName("AzurePublicCloud")
	var client *AzureClient
	if client, err = getAzureClient(env,
		c.credentials.SubscriptionId,
		c.credentials.ClientID,
		c.credentials.TennantID,
		c.credentials.ClientSecret); err != nil {
		return fmt.Errorf("Error trying to get Azure Client: %v", err)
	}
	c.azureClient = client
	return nil
}

func (c *Cluster) createCluster() error {
	var err error
	kubecfgDir, _ := ioutil.ReadDir(path.Join(c.outputDir, "kubeconfig"))
	kubecfg := path.Join(c.outputDir, "kubeconfig", kubecfgDir[0].Name())
	log.Printf("Setting kubeconfig env variable: kubeconfig path: %v.", kubecfg)
	os.Setenv("KUBECONFIG", kubecfg)
	log.Printf("Creating resurce group: %v.", c.resourceGroup)

	_, err = c.azureClient.EnsureResourceGroup(c.ctx, c.resourceGroup, c.location, nil)
	if err != nil {
		return fmt.Errorf("Could not ensure resource group: %v", err)
	}
	log.Printf("Validating deployment templates.")
	if _, err := c.azureClient.ValidateDeployment(
		c.ctx, c.resourceGroup, c.name, &c.templateJSON, &c.parametersJSON,
	); err != nil {
		return fmt.Errorf("Template invalid: %v", err)
	}
	log.Printf("Deploying cluster %v in resource group %v.", c.name, c.resourceGroup)
	if _, err := c.azureClient.DeployTemplate(
		c.ctx, c.resourceGroup, c.name, &c.templateJSON, &c.parametersJSON,
	); err != nil {
		return fmt.Errorf("Cannot deploy: %v", err)
	}
	return nil

}

func buildHyperKube() error {
        docker_user := os.Getenv("DOCKER_USER")
        docker_pass := os.Getenv("DOCKER_PASS")
        username :=  "--username=" + docker_user
        password :=  "--password=" + docker_pass
        if err := control.FinishRunning(exec.Command("docker","login",username,password)); err != nil {
                return err
        }

	os.Setenv("VERSION", "1.1")
        os.Setenv("REGISTRY", "atuvenie")
        cwd, _ := os.Getwd()
        log.Printf("CWD %v", cwd)
	if err := control.FinishRunning(exec.Command("hack/dev-push-hyperkube.sh")); err != nil {
                return err
        }
	return nil
}

func (c Cluster) uploadZip(zipPath string) error {

//	var accountName string = "k8szipstorage"
//	var accountKey string = "sLDa0vyBO39sGi4KHpZkTANLw1lRd3GikI2G/3xrdJ0C8v0XDdlpeFWjbljIxTcfAXgiQnOVTaLYztCbbP72SQ=="
	credential := azblob.NewSharedKeyCredential(c.credentials.StorageAccountName, c.credentials.StorageAccountKey)
	p := azblob.NewPipeline(credential, azblob.PipelineOptions{})

	var containerName string = "mystoragecontainer"

	URL, _ := url.Parse(
		fmt.Sprintf("https://%s.blob.core.windows.net/%s", c.credentials.StorageAccountName, containerName))

	containerURL := azblob.NewContainerURL(*URL, p)

	blobURL := containerURL.NewBlockBlobURL("v1.0int.zip")
	file, _ := os.Open(zipPath)
	_, err := azblob.UploadFileToBlockBlob(context.Background(), file, blobURL, azblob.UploadToBlockBlobOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (c Cluster) buildWinZip() error {

	zip_path := path.Join(os.Getenv("HOME"),"v1.0int.zip")
	build_script_path := path.Join(os.Getenv("GOPATH"),"src","k8s.io", "test-infra","kubetest","build-win.sh")
        if err := control.FinishRunning(exec.Command(build_script_path, zip_path)); err != nil {
		return err
	}
        if err := c.uploadZip(zip_path) ; err != nil {
		return err
	}
	return nil
}

func (c Cluster) Up() error {
	err := c.buildWinZip()
        if err != nil {
		return fmt.Errorf("Problem building hyperkube %v", err)
 	}
/*	var err error
	if c.apiModelPath == "" {
		err = c.generateTemplate()
		if err != nil {
			return fmt.Errorf("Failed to generate apiModel: %v", err)
		}
	}
	err = c.getAcsEngine(1)
	if err != nil {
		return fmt.Errorf("Failed to get ACS Engine binary: %v", err)
	}
	err = c.generateARMTemplates()
	if err != nil {
		return fmt.Errorf("Failed to generate ARM templates: %v", err)
	}
	err = c.loadARMTemplates()
	if err != nil {
		return fmt.Errorf("Error loading ARM templates: %v", err)
	}
	err = c.createCluster()
	if err != nil {
		return fmt.Errorf("Error creating cluster: %v", err)
	}
*/
	return nil
}

func (c Cluster) Down() error {
	log.Printf("Deleting resource group: %v.", c.resourceGroup)
//	return c.azureClient.DeleteResourceGroup(c.ctx, c.resourceGroup)
	return nil
}

func (c Cluster) DumpClusterLogs(localPath, gcsPath string) error {
	return nil
}

func (c Cluster) GetClusterCreated(clusterName string) (time.Time, error) {
	return time.Time{}, errors.New("not implemented")
}

func (c Cluster) TestSetup() error {
	return nil
}

func (c Cluster) IsUp() error {
	return isUp(c)
}
