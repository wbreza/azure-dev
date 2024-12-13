using System;
using System.Threading.Tasks;
using Grpc.Net.Client;
using Azdext;

class Program
{
    static async Task Main(string[] args)
    {
        Console.WriteLine("Hello from C#");
        Console.WriteLine();

        var azdServerAddress = "http://" + System.Environment.GetEnvironmentVariable("AZD_SERVER");
        using var channel = GrpcChannel.ForAddress(azdServerAddress);

        var deploymentClient = new DeploymentService.DeploymentServiceClient(channel);
        var promptClient = new PromptService.PromptServiceClient(channel);
        var envClient = new EnvironmentService.EnvironmentServiceClient(channel);

        var azdScope = new AzureScope();

        var getContextResponse = await deploymentClient.GetDeploymentContextAsync(new EmptyRequest());
        var azdContext = getContextResponse.AzureContext;

        var getSubscriptionResponse = await promptClient.PromptSubscriptionAsync(new PromptSubscriptionRequest());
        azdScope.SubscriptionId = getSubscriptionResponse.Subscription.Id;

        Console.WriteLine("Selected Subscription: " +  azdScope.SubscriptionId);
        Console.WriteLine();

        var getLocationResponse = await promptClient.PromptLocationAsync(new PromptLocationRequest() { 
            AzureContext = azdContext
        });
        azdScope.Location = getLocationResponse.Location.Name;

        Console.WriteLine("Selected Location: " + azdScope.Location);
        Console.WriteLine();

        var getResourceGroupResponse = await promptClient.PromptResourceGroupAsync(new PromptResourceGroupRequest() { AzureContext = azdContext });
        azdScope.ResourceGroup = getResourceGroupResponse.ResourceGroup.Name;

        Console.WriteLine("Selected Resource Group: " + azdScope.ResourceGroup);
        Console.WriteLine();

        var resourceOptions = new PromptResourceOptions();
        resourceOptions.ResourceType = "Microsoft.CognitiveServices/accounts";
        resourceOptions.Kinds.AddRange(new List<string>() { "OpenAI", "AIServices", "CognitiveServices" });
        resourceOptions.ResourceTypeDisplayName = "Azure AI Service";
        var promptAiResource = await promptClient.PromptSubscriptionResourceAsync(new PromptSubscriptionResourceRequest()
        {
            AzureContext = azdContext,
            Options = resourceOptions,
        });

        Console.WriteLine("Selected AI Resource: " + promptAiResource.Resource.Name);
        Console.WriteLine();

        var getEnvResponse = await envClient.GetCurrentAsync(new EmptyRequest());
        var azdEnv = getEnvResponse.Environment;

        var getValuesResponse = await envClient.GetValuesAsync(new GetEnvironmentRequest()
        {
            Name = azdEnv.Name,
        });

        Console.WriteLine(getValuesResponse.KeyValues.ToString());
    }
}
