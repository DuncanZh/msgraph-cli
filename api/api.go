package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	auth "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"sync"
)

type GraphAPI struct {
	credential      *azidentity.ClientSecretCredential
	userClient      *msgraphsdk.GraphServiceClient
	graphUserScopes []string
}

func NewGraphAPI() *GraphAPI {
	g := &GraphAPI{}
	return g
}

func (g *GraphAPI) InitializeGraphForUserAuth(clientId string, clientSecret string, tenantId string) error {
	g.graphUserScopes = []string{"https://graph.microsoft.com/.default"}

	credential, err := azidentity.NewClientSecretCredential(tenantId, clientId, clientSecret, nil)
	if err != nil {
		return err
	}

	g.credential = credential

	// Create an auth provider using the credential
	authProvider, err := auth.NewAzureIdentityAuthenticationProviderWithScopes(credential, g.graphUserScopes)
	if err != nil {
		return err
	}

	// Create a request adapter using the auth provider
	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return err
	}

	// Create a Graph client using request adapter
	client := msgraphsdk.NewGraphServiceClient(adapter)
	g.userClient = client

	return nil
}

func (g *GraphAPI) GetUsers() []map[string]interface{} {
	/*
		query := users.UsersRequestBuilderGetQueryParameters{
			Select: []string{"displayName", "id"},
		}

		result, err := g.userClient.Users().Get(context.Background(), &users.UsersRequestBuilderGetRequestConfiguration{
			QueryParameters: &query,
		})
		if err != nil {
			g.printOdataError(err)
			return
		}
	*/
	result, err := g.userClient.Users().Get(context.Background(), nil)
	if err != nil {
		g.printOdataError(err)
		return nil
	}

	var results []map[string]interface{}
	pageIterator, err := msgraphcore.NewPageIterator[models.Userable](result, g.userClient.GetAdapter(), models.CreateUserCollectionResponseFromDiscriminatorValue)
	err = pageIterator.Iterate(context.Background(), func(user models.Userable) bool {
		results = append(results, user.GetBackingStore().Enumerate())
		return true
	})
	return results
}

func (g *GraphAPI) GetAuthenticationById(userId string) map[string]interface{} {
	result, err := g.userClient.Users().ByUserId(userId).Authentication().Methods().Get(context.Background(), nil)
	if err != nil {
		g.printOdataError(err)
		return nil
	}
	g.userClient.Users().GetByIds()
	return result.GetBackingStore().Enumerate()
}

func (g *GraphAPI) GetAuthenticationByIds(userIds []string) *map[string][]map[string]interface{} {
	batch := msgraphcore.NewBatchRequest(g.userClient.GetAdapter())
	stepMap := make(map[string]msgraphcore.BatchItem)
	result := make(map[string][]map[string]interface{})

	for _, id := range userIds {
		request, err := g.userClient.Users().ByUserId(id).Authentication().Methods().ToGetRequestInformation(context.Background(), nil)
		if err != nil {
			g.printOdataError(err)
			return nil
		}
		step, err := batch.AddBatchRequestStep(*request)
		if err != nil {
			g.printOdataError(err)
			return nil
		}
		stepMap[id] = step
	}
	send, err := batch.Send(context.Background(), g.userClient.GetAdapter())
	if err != nil {
		g.printOdataError(err)
		return nil
	}

	for k, v := range stepMap {
		response, err := msgraphcore.GetBatchResponseById[models.AuthenticationMethodCollectionResponseable](send, *v.GetId(), models.CreateAuthenticationMethodCollectionResponseFromDiscriminatorValue)
		if err != nil {
			g.printOdataError(err)
			return nil
		}

		for _, v := range response.GetValue() {
			result[k] = append(result[k], v.GetBackingStore().Enumerate())
		}
	}

	return &result
}

func (g *GraphAPI) GetAuthenticationByIdsConcurrent(userIds []string, wg *sync.WaitGroup, lock *sync.Mutex, result *map[string][]interface{}) {
	defer wg.Done()

	batch := msgraphcore.NewBatchRequest(g.userClient.GetAdapter())
	stepMap := make(map[string]msgraphcore.BatchItem)

	for _, id := range userIds {
		request, err := g.userClient.Users().ByUserId(id).Authentication().Methods().ToGetRequestInformation(context.Background(), nil)
		if err != nil {
			g.printOdataError(err)
			return
		}
		step, err := batch.AddBatchRequestStep(*request)
		if err != nil {
			g.printOdataError(err)
			return
		}
		stepMap[id] = step
	}
	send, err := batch.Send(context.Background(), g.userClient.GetAdapter())
	if err != nil {
		g.printOdataError(err)
		return
	}

	for k, v := range stepMap {
		response, err := msgraphcore.GetBatchResponseById[models.AuthenticationMethodCollectionResponseable](send, *v.GetId(), models.CreateAuthenticationMethodCollectionResponseFromDiscriminatorValue)
		if err != nil {
			g.printOdataError(err)
			return
		}
		lock.Lock()
		for _, v := range response.GetValue() {
			(*result)[k] = append((*result)[k], v.GetBackingStore().Enumerate())
		}
		lock.Unlock()
	}
}

func (g *GraphAPI) GetResourceConcurrent(userIds []string, slice int, f func([]string, *sync.WaitGroup, *sync.Mutex, *map[string][]interface{})) map[string][]interface{} {
	result := make(map[string][]interface{})
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}

	i := 0
	for ; i < len(userIds)-slice; i += slice {
		wg.Add(1)
		f(userIds[i:i+slice], &wg, &lock, &result)
	}
	wg.Add(1)
	f(userIds[i:], &wg, &lock, &result)

	wg.Wait()
	return result
}

/*
func (g *GraphAPI) GetResourceByIdsConcurrent(userIds []string, resource string, wg *sync.WaitGroup, lock *sync.Mutex, result *map[string][]map[string]interface{}) {
	defer wg.Done()

	resources := strings.Split(resource, "/")
	for i := 0; i < len(resources); i++ {
		resources[i] = strings.Title(resources[i])
	}
	resource = strings.Join(resources, "")

	batch := msgraphcore.NewBatchRequest(g.userClient.GetAdapter())
	stepMap := make(map[string]msgraphcore.BatchItem, len(userIds))

	for _, id := range userIds {
		method := reflect.ValueOf(g.userClient.Users().ByUserId(id))
		for _, v := range resources {
			method = method.MethodByName(v).Call([]reflect.Value{})[0]
		}
		request := method.MethodByName("ToGetRequestInformation").Call([]reflect.Value{reflect.ValueOf(context.Background()), reflect.ValueOf((*users.ItemAuthenticationMethodsRequestBuilderGetRequestConfiguration)(nil))})[0].Interface().(*abstractions.RequestInformation)

		step, err := batch.AddBatchRequestStep(*request)
		if err != nil {
			g.printOdataError(err)
			return
		}
		stepMap[id] = step
	}
	send, err := batch.Send(context.Background(), g.userClient.GetAdapter())
	if err != nil {
		g.printOdataError(err)
		return
	}

	for k, v := range stepMap {
		response, err := msgraphcore.GetBatchResponseById[models.BaseItemCollectionResponseable](send, *v.GetId(), models.CreateBaseItemCollectionResponseFromDiscriminatorValue)
		if err != nil {
			g.printOdataError(err)
			return
		}
		lock.Lock()
		for _, v := range response.GetValue() {
			(*result)[k] = append((*result)[k], v.GetBackingStore().Enumerate())
		}
		lock.Unlock()
	}
}

func (g *GraphAPI) ConcurrentResource(userIds []string, resource string, slice int, f func([]string, string, *sync.WaitGroup, *sync.Mutex, *map[string][]map[string]interface{})) map[string][]map[string]interface{} {
	result := make(map[string][]map[string]interface{}, len(userIds))
	wg := sync.WaitGroup{}
	lock := sync.Mutex{}

	i := 0
	for ; i < len(userIds)-slice; i += slice {
		wg.Add(1)
		f(userIds[i:i+slice], resource, &wg, &lock, &result)
	}
	wg.Add(1)
	f(userIds[i:], resource, &wg, &lock, &result)

	wg.Wait()
	return result
}
*/

func (g *GraphAPI) IsInitiated() bool {
	return g.userClient != nil
}

func (g *GraphAPI) printOdataError(err error) {
	var ODataError *odataerrors.ODataError
	switch {
	case errors.As(err, &ODataError):
		var typed *odataerrors.ODataError
		errors.As(err, &typed)
		fmt.Printf("error: %s\n", typed.Error())
		if terr := typed.GetErrorEscaped(); terr != nil {
			fmt.Printf("code: %s\n", *terr.GetCode())
			fmt.Printf("msg: %s\n", *terr.GetMessage())
		}
	default:
		fmt.Printf("%T > error: %#v", err, err)
	}
}
