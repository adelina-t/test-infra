package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/azure-sdk-for-go-samples/resources"
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
	_, err := resources.CreateGroup(ctx, az.resourceGroupName)
	if err != nil {
		return err
	}

	return nil
}

func (az azure) createCluster() error {
	fmt.Printf("CREATING CLUSTER")
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
