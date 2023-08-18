package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/abiosoft/ishell"
	abstractions "github.com/microsoft/kiota-abstractions-go"
	auth "github.com/microsoft/kiota-authentication-azure-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"strings"
	"sync"
	"time"
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
			g.printError(err)
			return
		}
	*/
	result, err := g.userClient.Users().Get(context.Background(), nil)
	if err != nil {
		g.printError(err)
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
		g.printError(err)
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
			g.printError(err)
			return nil
		}
		step, err := batch.AddBatchRequestStep(*request)
		if err != nil {
			g.printError(err)
			return nil
		}
		stepMap[id] = step
	}
	send, err := batch.Send(context.Background(), g.userClient.GetAdapter())
	if err != nil {
		g.printError(err)
		return nil
	}

	for k, v := range stepMap {
		response, err := msgraphcore.GetBatchResponseById[models.AuthenticationMethodCollectionResponseable](send, *v.GetId(), models.CreateAuthenticationMethodCollectionResponseFromDiscriminatorValue)
		if err != nil {
			g.printError(err)
			return nil
		}

		for _, v := range response.GetValue() {
			result[k] = append(result[k], v.GetBackingStore().Enumerate())
		}
	}

	return &result
}

func (g *GraphAPI) GetAuthenticationByIdsConcurrent(lock *sync.Mutex, input chan []string, output chan int, pause chan bool, result *map[string][]interface{}) {
out:
	for userIds := range input {
		batch := msgraphcore.NewBatchRequest(g.userClient.GetAdapter())
		stepMap := make(map[string]msgraphcore.BatchItem)

		for _, id := range userIds {
			if stepMap[id] != nil {
				output <- 1
				continue
			}

			request, err := g.userClient.Users().ByUserId(id).Authentication().Methods().ToGetRequestInformation(context.Background(), nil)
			if err != nil {
				g.printError(err)
				return
			}

			step, err := batch.AddBatchRequestStep(*request)
			if err != nil {
				g.printError(err)
				return
			}

			stepMap[id] = step
		}

		for len(pause) > 0 {
			// Wait if paused
		}

		resp, err := batch.Send(context.Background(), g.userClient.GetAdapter())
		if err != nil {
			g.printError(err)
			return
		}

		if len(pause) > 0 {
			// Sent batch during pause, abandon result
			input <- userIds
			continue
		}

		for k, v := range stepMap {
			response, err := msgraphcore.GetBatchResponseById[models.AuthenticationMethodCollectionResponseable](resp, *v.GetId(), models.CreateAuthenticationMethodCollectionResponseFromDiscriminatorValue)
			if err != nil {
				if strings.Contains(err.Error(), "429") {
					input <- userIds
					if len(pause) == 0 {
						pause <- true
						time.Sleep(30 * time.Second)
						<-pause
					}
					continue out
				} else {
					g.printError(err)
				}
			}
			lock.Lock()
			for _, v := range response.GetValue() {
				(*result)[k] = append((*result)[k], v.GetBackingStore().Enumerate())
			}
			lock.Unlock()
			output <- 1
		}
	}
}

func (g *GraphAPI) GetResourceConcurrent(c *ishell.Context, userIds []string, n int, slice int, f func(*sync.Mutex, chan []string, chan int, chan bool, *map[string][]interface{})) map[string][]interface{} {
	result := make(map[string][]interface{})
	lock := sync.Mutex{}

	input := make(chan []string, len(userIds)/slice+1)
	output := make(chan int, len(userIds))
	pause := make(chan bool, n)

	for i := 0; i < n; i++ {
		go f(&lock, input, output, pause, &result)
	}

	i := 0
	for ; i < len(userIds)-slice; i += slice {
		input <- userIds[i : i+slice]
	}
	input <- userIds[i:]

	c.ProgressBar().Start()

	for len(output) != len(userIds) {
		percent := len(output) * 100 / len(userIds)

		if len(pause) > 0 {
			c.ProgressBar().Suffix(fmt.Sprint(" ", len(output), "/", len(userIds), " (", percent, "%) PAUSED: Too many requests, please wait..."))
			c.ProgressBar().Progress(percent)
		} else {
			c.ProgressBar().Suffix(fmt.Sprint(" ", len(output), "/", len(userIds), " (", percent, "%)", "                                            "))
			c.ProgressBar().Progress(percent)
		}
	}

	c.ProgressBar().Suffix(fmt.Sprint(" ", len(userIds), "/", len(userIds), " (", 100, "%)", "                                          "))
	c.ProgressBar().Progress(100)
	c.ProgressBar().Stop()

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
			g.printError(err)
			return
		}
		stepMap[id] = step
	}
	send, err := batch.Send(context.Background(), g.userClient.GetAdapter())
	if err != nil {
		g.printError(err)
		return
	}

	for k, v := range stepMap {
		response, err := msgraphcore.GetBatchResponseById[models.BaseItemCollectionResponseable](send, *v.GetId(), models.CreateBaseItemCollectionResponseFromDiscriminatorValue)
		if err != nil {
			g.printError(err)
			return
		}
		lock.Lock()
		for _, v := range response.GetValue() {
			(*result)[k] = append((*result)[k], v.GetBackingStore().Enumerate())
		}
		lock.Unlock()
	}
}

func (g *GraphAPI) GetResourceConcurrent(c *ishell.Context, userIds []string, n int, slice int, f func(*sync.Mutex, chan []string, *map[string][]interface{})) map[string][]interface{} {
	result := make(map[string][]interface{})
	lock := sync.Mutex{}

	input := make(chan []string, n)

	for i := 0; i < n; i++ {
		go f(&lock, input, &result)
	}

	c.ProgressBar().Start()
	c.ProgressBar().Progress(0)

	i := 0
	for ; i < len(userIds)-slice; i += slice {
		input <- userIds[i : i+slice]

		percent := i * 100 / len(userIds)
		c.ProgressBar().Suffix(fmt.Sprint(" ", i, "/", len(userIds), " (", percent, "%)"))
		c.ProgressBar().Progress(percent)
	}
	input <- userIds[i:]

	for len(input) > 0 {
	}

	c.ProgressBar().Suffix(fmt.Sprint(" ", len(userIds), "/", len(userIds), " (", 100, "%)"))
	c.ProgressBar().Progress(100)
	c.ProgressBar().Stop()

	return result
}
*/

func (g *GraphAPI) IsInitiated() bool {
	return g.userClient != nil
}

func (g *GraphAPI) printError(err error) {
	var ODataError *odataerrors.ODataError
	var ApiError *abstractions.ApiError
	switch {
	case errors.As(err, &ODataError):
		errors.As(err, &ODataError)
		fmt.Printf("error: %s\n", ODataError.Error())
		if terr := ODataError.GetErrorEscaped(); terr != nil {
			fmt.Printf("code: %s\n", *terr.GetCode())
			fmt.Printf("msg: %s\n", *terr.GetMessage())
		}
	case errors.As(err, &ApiError):
		errors.As(err, &ApiError)
		fmt.Printf("error: %s\n", ApiError.Error())
		fmt.Printf("code: %d\n", ApiError.ResponseStatusCode)
		fmt.Printf("msg: %s\n", ApiError.Message)
		fmt.Printf("header: %v\n", ApiError.ResponseHeaders)
	default:
		fmt.Printf("%T > error: %#v", err, err)
	}
}
