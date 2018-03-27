package main

import (
	"encoding/json"
	"flag"
	"fmt"
	ioutil "io/ioutil"
	"log"
	"os"
	"path"
	"time"

	"github.com/Azure/acs-engine/pkg/acsengine"
	"github.com/Azure/acs-engine/pkg/acsengine/transform"

	api "github.com/Azure/acs-engine/pkg/api"
	azure "github.com/Azure/go-autorest/autorest/azure"
)

var (
	// azure specific flags
	azResourceName      = flag.String("azResName", "", "Azure Resource Name")
	azResourceGroupName = flag.String("azResGrName", "", "Azure Resource Group Name")
	azLocation          = flag.String("azLocation", "westus2", "Azure ACS location")
	azMasterVmSize      = flag.String("azMasterVMSize", "Standard_D2s_v3", "Azure Master VM size")
	azAgentVmSize       = flag.String("azAgentVMSize", "Standard_D2s_v3", "Azure Agent VM size")
	azAdminUsername     = flag.String("azAdminUsername", "azureuser", "Admin username")
	azAdminPassword     = flag.String("azAdminPassword", "Passw0rdAdmin", "Admin password")
	azAgentPoolCount    = flag.Int("azAgentPoolCount", 2, "Azure Agent Pool Count")
	azTemplatePath      = flag.String("azTemplatePath", "", "Azure Template Name")
	ClientID            = "03fee577-e432-49a0-965b-69d12177e071"
	ClientSecret        = "27d1af4f-4e04-4580-b122-9ac125adae3a"
	SubscriptionId      = "7fe76de7-a6e6-491a-b482-449cec7c91fd"
)

type Cluster struct {
	client           AzureClient
	clientId         string
	clientSecret     string
	subscriptionId   string
	apiVersion       string
	containerService *api.ContainerService
	location         string
	resourceGroup    string
	name             string
	dnsPrefix        string
	apiModelPath     string
	templateJSON     map[string]interface{}
	parametersJSON   map[string]interface{}
	outputDir        string
	sshPublicKeyPath string
	adminUsername    string
	adminPassword    string
	masterVMSize     string
	agentVMSize      string
}

func (c *Cluster) getARMClient() error {
	env, err := azure.EnvironmentFromName("AzurePublicCloud")
	var client *AzureClient
	if client, err = getAzureClient(env,
		c.subscriptionId,
		c.clientId,
		c.clientSecret); err != nil {
		return fmt.Errorf("Error trying to get Azure Client: %v", err)
	}
	//client.AddAcceptLanguages([]string{"en-us"})
	c.client = client
	return nil
}

func (c *Cluster) fillModelFromParams() {
	// overwrite options from cmdline for name and resource group
	c.containerService.Name = c.name
	c.containerService.Location = c.location
	c.containerService.Tags = map[string]string{
		"creationDate": time.Now().String(),
		"type":         "preview",
	}
}

func (c *Cluster) getTemplates() error {
	var err error
	apiLoader := &api.Apiloader{}
	c.containerService, c.apiVersion, err = apiLoader.LoadContainerServiceFromFile(
		c.apiModelPath, false, false, nil)
	if err != nil {
		return fmt.Errorf("Can't load api from file: %v, %v", c.apiModelPath, err.Error())
	}
	fmt.Printf("Loaded container service: %v", c.containerService)
	c.fillModelFromParams()

	ctxt := acsengine.Context{}
	templateGenerator, err := acsengine.InitializeTemplateGenerator(ctxt, true)
	if err != nil {
		return fmt.Errorf("Can't load template generator: %v", err.Error())
	}
	template, parameters, certs, err := templateGenerator.GenerateTemplate(c.containerService,
		acsengine.DefaultGeneratorCode, false)
	if err != nil {
		return fmt.Errorf("Can't generate templates: %v", err.Error())
	}

	if template, err = transform.PrettyPrintArmTemplate(template); err != nil {
		return fmt.Errorf("Can't pretty print template: %v", err.Error())
	}

	var parametersFile string
	if parametersFile, err = transform.BuildAzureParametersFile(parameters); err != nil {
		return fmt.Errorf("Can't pretty print parameters file: %v", err.Error())
	}

	writer := &acsengine.ArtifactWriter{}
	if err = writer.WriteTLSArtifacts(c.containerService, c.apiVersion, template, parametersFile, c.outputDir, certs, false); err != nil {
		log.Fatalf("error writing artifacts: %s \n", err.Error())
	}

	err = json.Unmarshal([]byte(template), &c.templateJSON)
	if err != nil {
		return fmt.Errorf("Error unmarshall template %v", err.Error())
	}

	err = json.Unmarshal([]byte(parameters), &c.parametersJSON)
	if err != nil {
		return fmt.Errorf("Error unmarshall parameters %v", err.Error())
	}
	return nil
}

func (c Cluster) createCluster() error {
	var err error

	err = c.getARMClient()
	if err != nil {
		return fmt.Errorf("Failed to get ARM  Client %v", err)
	}
	err = c.getTemplates()
	if err != nil {
		return fmt.Errorf("Failed to get templates %v", err)
	}
	kubecfg_dir, _ := ioutil.ReadDir(path.Join(c.outputDir, "kubeconfig"))
	kubecfg := path.Join(c.outputDir, "kubeconfig", kubecfg_dir[0].Name())
	fmt.Printf("Setting kubeconfig env variable: kubeconfig path: %v", kubecfg)
	os.Setenv("KUBECONFIG", kubecfg)
	_, err = c.client.EnsureResourceGroup(c.resourceGroup, c.location, nil)

	if err != nil {
		return fmt.Errorf("Could not ensure resource group: %v", err)
	}

	if res, err := c.client.DeployTemplate(
		c.resourceGroup,
		c.name,
		c.templateJSON,
		c.parametersJSON,
		nil); err != nil {
		if res != nil && res.Response.Response != nil && res.Body != nil {
			defer res.Body.Close()
			body, _ := ioutil.ReadAll(res.Body)
			fmt.Printf(string(body))
		}
		fmt.Printf("Cannot deploy: %v", err)
	}
	return nil
}

func (c Cluster) Up() error {
	var err error

	err = c.createCluster()
	if err != nil {
		return fmt.Errorf("Failed to deploy cluster", err)
	}
	return nil
}

func (c Cluster) Down() error {
	return nil
}

func (c Cluster) DumpClusterLogs(localPath, gcsPath string) error {
	return nil
}

func (c Cluster) GetClusterCreated(clusterName string) (time.Time, error) {
	return time.Now(), nil
}

func (c Cluster) TestSetup() error {
	return nil
}

func (c Cluster) IsUp() error {
	return isUp(c)
}

func newAzure() (*Cluster, error) {
	if *azResourceName == "" {
		return nil, fmt.Errorf("--azResName must be set to a valid cluster Name")
	}

	if *azResourceGroupName == "" {
		return nil, fmt.Errorf("--azResGrName must be set to a valid resource group name")
	}

	tempdir, _ := ioutil.TempDir(os.Getenv("HOME"), "acs")
	return &Cluster{
		clientId:         ClientID,
		clientSecret:     ClientSecret,
		subscriptionId:   SubscriptionId,
		apiModelPath:     *azTemplatePath,
		name:             *azResourceName,
		dnsPrefix:        *azResourceName + "-agent",
		location:         *azLocation,
		resourceGroup:    *azResourceGroupName,
		outputDir:        tempdir,
		sshPublicKeyPath: os.Getenv("HOME") + "/.ssh/id_rsa.pub",
	}, nil
}
