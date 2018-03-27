package main

import (
	"time"

	"github.com/Azure/acs-engine/pkg/acsengine"
	"github.com/Azure/azure-sdk-for-go/arm/authorization"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/disk"
	"github.com/Azure/azure-sdk-for-go/arm/graphrbac"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
)

type AzureClient struct {
	acceptLanguages               []string
	environment                   azure.Environment
	subscriptionID                string
	authorizationClient           authorization.RoleAssignmentsClient
	deploymentsClient             resources.DeploymentsClient
	deploymentOperationsClient    resources.DeploymentOperationsClient
	resourcesClient               resources.GroupClient
	storageAccountsClient         storage.AccountsClient
	interfacesClient              network.InterfacesClient
	groupsClient                  resources.GroupsClient
	providersClient               resources.ProvidersClient
	virtualMachinesClient         compute.VirtualMachinesClient
	virtualMachineScaleSetsClient compute.VirtualMachineScaleSetsClient
	disksClient                   disk.DisksClient

	applicationsClient      graphrbac.ApplicationsClient
	servicePrincipalsClient graphrbac.ServicePrincipalsClient
}

func (az *AzureClient) DeployTemplate(resourceGroupName, deploymentName string, template map[string]interface{}, parameters map[string]interface{}, cancel <-chan struct{}) (*resources.DeploymentExtended, error) {
	deployment := resources.Deployment{
		Properties: &resources.DeploymentProperties{
			Template:   &template,
			Parameters: &parameters,
			Mode:       resources.Incremental,
		},
	}

	log.Infof("Starting ARM Deployment (%s). This will take some time...", deploymentName)

	resChan, errChan := az.deploymentsClient.CreateOrUpdate(
		resourceGroupName,
		deploymentName,
		deployment,
		cancel)
	if err := <-errChan; err != nil {
		return nil, err
	}
	res := <-resChan

	log.Infof("Finished ARM Deployment (%s).", deploymentName)

	return &res, nil
}

func (az *AzureClient) EnsureResourceGroup(name, location string, managedBy *string) (resourceGroup *resources.Group, err error) {
	var tags *map[string]*string
	group, err := az.groupsClient.Get(name)
	if err == nil {
		tags = group.Tags
	}

	response, err := az.groupsClient.CreateOrUpdate(name, resources.Group{
		Name:      &name,
		Location:  &location,
		ManagedBy: managedBy,
		Tags:      tags,
	})
	if err != nil {
		return &response, err
	}

	return &response, nil
}

func getOAuthConfig(env azure.Environment, subscriptionID string) (*adal.OAuthConfig, string, error) {
	tenantID, err := acsengine.GetTenantID(env, subscriptionID)
	if err != nil {
		return nil, "", err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, tenantID)
	if err != nil {
		return nil, "", err
	}

	return oauthConfig, tenantID, nil
}

func getAzureClient(env azure.Environment, subscriptionID, clinetID, clientSecret string) (*AzureClient, error) {
	oauthConfig, tenantID, err := getOAuthConfig(env, subscriptionID)
	if err != nil {
		return nil, err
	}

	armSpt, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, env.ServiceManagementEndpoint)
	if err != nil {
		return nil, err
	}
	graphSpt, err := adal.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, env.GraphEndpoint)
	if err != nil {
		return nil, err
	}
	graphSpt.Refresh()

	return getClient(env, subscriptionID, tenantID, armSpt, graphSpt), nil
}

func getClient(env azure.Environment, subscriptionID, tenantID string, armSpt *adal.ServicePrincipalToken, graphSpt *adal.ServicePrincipalToken) *AzureClient {
	c := &AzureClient{
		environment:    env,
		subscriptionID: subscriptionID,

		authorizationClient:           authorization.NewRoleAssignmentsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		deploymentsClient:             resources.NewDeploymentsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		deploymentOperationsClient:    resources.NewDeploymentOperationsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		resourcesClient:               resources.NewGroupClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		storageAccountsClient:         storage.NewAccountsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		interfacesClient:              network.NewInterfacesClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		groupsClient:                  resources.NewGroupsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		providersClient:               resources.NewProvidersClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		virtualMachinesClient:         compute.NewVirtualMachinesClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		virtualMachineScaleSetsClient: compute.NewVirtualMachineScaleSetsClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),
		disksClient:                   disk.NewDisksClientWithBaseURI(env.ResourceManagerEndpoint, subscriptionID),

		applicationsClient:      graphrbac.NewApplicationsClientWithBaseURI(env.GraphEndpoint, tenantID),
		servicePrincipalsClient: graphrbac.NewServicePrincipalsClientWithBaseURI(env.GraphEndpoint, tenantID),
	}

	authorizer := autorest.NewBearerAuthorizer(armSpt)
	c.authorizationClient.Authorizer = authorizer
	c.deploymentsClient.Authorizer = authorizer
	c.deploymentOperationsClient.Authorizer = authorizer
	c.resourcesClient.Authorizer = authorizer
	c.storageAccountsClient.Authorizer = authorizer
	c.interfacesClient.Authorizer = authorizer
	c.groupsClient.Authorizer = authorizer
	c.providersClient.Authorizer = authorizer
	c.virtualMachinesClient.Authorizer = authorizer
	c.virtualMachineScaleSetsClient.Authorizer = authorizer
	c.disksClient.Authorizer = authorizer

	c.deploymentsClient.PollingDelay = time.Second * 5
	c.resourcesClient.PollingDelay = time.Second * 5

	graphAuthorizer := autorest.NewBearerAuthorizer(graphSpt)
	c.applicationsClient.Authorizer = graphAuthorizer
	c.servicePrincipalsClient.Authorizer = graphAuthorizer

	return c
}
