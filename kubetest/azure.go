package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/Azure-Samples/azure-sdk-for-go-samples/resources"
	"github.com/azure-sdk-for-go-samples/helpers"
	"github.com/azure-sdk-for-go-samples/iam"
	"github.com/azure-sdk-for-go/profiles/latest/containerservice/mgmt/containerservice"
	"github.com/go-autorest/autorest"
	"github.com/go-autorest/autorest/to"
)

var (
	// azure specific flags
	azResourceName      = flag.String("az-resName", "", "Azure Resource Name")
	azResourceGroupName = flag.String("az-resGrName", "", "Azure Resource Group Name")
	azLocation          = flag.String("az-location", "westus2", "Azure ACS location")
	azMasterVmSize      = flag.String("az-masterVMSize", "Standard_D2s_v3", "Azure Master VM size")
	azAgentVmSize       = flag.String("az-agentVMSize", "Standard_D2s_v3", "Azure Agent VM size")
	azAdminUsername     = flag.String("az-adminUsername", "azureuser", "Admin username")
	azAdminPassword     = flag.String("az-adminPassword", "Passw0rdAdmin", "Admin password")
	azAgentPoolCount    = flag.Int("az-agentPoolCount", 2, "Azure Agent Pool Count")
)

type azure struct {
	resourceName      string
	resourceGroupName string
	location          string
	masterVmSize      string
	agentVmSize       string
	adminUsername     string
	adminPassword     string
	username          string
	sshPublicKeyPath  string
	clientID          string
	clientSecret      string
	agentPoolCount    int32
	dnsPrefixMaster   string
	dnsPrefixAgent    string
}

func (az azure) createResourceGroup(ctx context.Context) error {
	groupsClient := resources.getGroupsClient()
	log.Println(fmt.Sprintf("creating resource group '%s' on location: %v", az.resourceGroupName, az.location))
	err := groupsClient.CreateOrUpdate(
		ctx,
		az.resourceGroupName,
		resources.Group{
			Location: to.StringPtr(az.location),
		})
	if err != nil {
		return err
	}

	return nil
}

func getACSClient() (containerservice.ContainerServicesClient, error) {
	token, err := iam.GetResourceManagementToken(iam.AuthGrantType())
	if err != nil {
		return containerservice.ContainerServicesClient{}, fmt.Errorf("cannot get token: %v", err)
	}

	acsClient := containerservice.NewContainerServicesClient(helpers.SubscriptionID())
	acsClient.Authorizer = autorest.NewBearerAuthorizer(token)
	acsClient.AddToUserAgent(helpers.UserAgent())
	return acsClient, nil
}

func (az azure) createCluster() error {
	fmt.Printf("CREATING CLUSTER")

	var sshKeyData string
	if _, err = os.Stat(sshPublicKeyPath); err == nil {
		sshBytes, err := ioutil.ReadFile(az.sshPublicKeyPath)
		if err != nil {
			log.Fatalf("failed to read SSH key data: %v", err)
		}
		sshKeyData = string(sshBytes)
	}

	acsClient, err := getACSClient()
	if err != nil {
		return c, fmt.Errorf("cannot get ACS client: %v", err)
	}

	future, err := acsClient.CreateOrUpdate(
		ctx,
		az.resourceGroupName,
		az.resourceName,
		containerservice.ContainerService{
			Name:     &az.resourceName,
			Location: &az.location,
			Properties: &containerservice.Properties{
				OrchestratorProfile: &containerservice.OrchestratorProfileType{
					OrchestratorType: containerservice.Kubernetes,
				},
				MasterProfile: &containerservice.MasterProfile{
					VMSize:    containerservice.StandardD2sV3,
					DNSPrefix: to.StringPtr(az.dnsPrefixMaster),
				},
				LinuxProfile: &containerservice.LinuxProfile{
					AdminUsername: to.StringPtr(az.adminUsername),
					SSH: &containerservice.SSHConfiguration{
						PublicKeys: &[]containerservice.SSHPublicKey{
							{
								KeyData: to.StringPtr(sshKeyData),
							},
						},
					},
				},
				AgentPoolProfiles: &[]containerservice.AgentPoolProfile{
					{
						Count:     to.Int32Ptr(az.agentPoolCount),
						Name:      to.StringPtr("agentpool1"),
						VMSize:    containerservice.StandardD2sV3,
						OsType:    containerservice.Windows,
						DNSPrefix: to.StringPtr(az.dnsPrefixAgent),
					},
				},
				WindowsProfile: &containerservice.WindowsProfile{
					AdminUsername: to.StringPtr(az.adminUsername),
					AdminPassword: to.StringPtr(az.adminPassword),
				},
				ServicePrincipalProfile: &containerservice.ServicePrincipalProfile{
					ClientID: to.StringPtr(az.clientID),
					Secret:   to.StringPtr(az.clientSecret),
				},
			},
		},
	)
	if err != nil {
		return c, fmt.Errorf("cannot create AKS cluster: %v", err)
	}

	err = future.WaitForCompletion(ctx, aksClient.Client)
	if err != nil {
		return c, fmt.Errorf("cannot get the AKS cluster create or update future response: %v", err)
	}

	return nil
}

func (az azure) Up() error {
	ctx := context.Background()
	err := az.createResourceGroup(ctx)

	if err != nil {
		return fmt.Errorf("Failed to build resource group: ", err)
	}

	err2 := az.createCluster()
	if err2 != nil {
		return fmt.Errorf("FAiled to deploy cluster", err)
	}
	return nil
}

func (az azure) Down() error {
	return nil
}

func (az azure) DumpClusterLogs(localPath, gcsPath string) error {
	return nil
}

func (az azure) GetClusterCreated(clusterName string) (time.Time, error) {
	return time.Now(), nil
}

func (az azure) TestSetup() error {
	return nil
}

func (az azure) IsUp() error {
	return isUp(az)
}

func newAzure() (*azure, error) {
	if *azResourceName == "" {
		return nil, fmt.Errorf("--az-resName must be set to a valid cluster Name")
	}

	if *azResourceGroupName == "" {
		return nil, fmt.Errorf("--az-resGrName must be set to a valid resource group name")
	}
	return &azure{
		resourceName:      *azResourceName,
		resourceGroupName: *azResourceGroupName,
		location:          *azLocation,
		masterVmSize:      *azMasterVmSize,
		agentVmSize:       *azAgentVmSize,
		adminUsername:     *azAdminUsername,
		adminPassword:     *azAdminPassword,
		agentPoolCount:    int32(*azAgentPoolCount),
		sshPublicKeyPath:  os.Getenv("HOME") + "/.ssh/id_rsa.pub",
		clientID:          os.Getenv("AZ_CLIENT_ID"),
		clientSecret:      os.Getenv("AZ_CLIENT_SECRET"),
		dnsPrefixAgent:    *azResourceName + "-agent",
		dnsPrefixMaster:   *azResourceName + "-master",
	}, nil
}
