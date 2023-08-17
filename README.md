# Microsoft Graph API CLI

`msgraph-cli` is a command-line interface (CLI) tool designed to interact with the Microsoft Graph API writen in Go. It provides an interactive shell, allowing users to authenticate, fetch users, and retrieve specified resources.

## Features

- Authenticate with Microsoft Graph API using credentials or a file
- Fetch users from the Graph API
- Retrieve specified resources concurrently
- Interactive shell for ease of use
- Store results to JSON files for future use

## Usage

Run the program by executing:

```
go run main.go
```

### Commands

#### 1. `auth`

Authenticate with the Microsoft Graph API with secret. The `auth` command accepts literals or a JSON credential file containing the `clientId`, `clientSecret`, and `tenantId`.

```
auth <credential_file>
auth <client_id> <client_secret> <tenant_id>
```

#### 2. `get users`

Fetch all users and dump to `output_file`.

```
get users <output_file>
```

#### 3. `get resource`

Fetch specified resources by user ids from `input_file` and dump to `output_file`. The input file must be a valid JSON array containing users with ids. Refer to the output of `get users`.

```
get resource <type> <input_file> <output_file>
```

## Examples

**Authenticate with a credentials file**:

```
auth credentials.json
```

*credentials.json*

```json
{
    "clientId": "xxx",
    "clientSecret": "xxx",
    "tenantId": "xxx"
}
```

**Authenticate with client ID, secret, and tenant ID**:

```
auth my_client_id my_client_secret my_tenant_id
```

**Get users**:

```
get users users.json
```

*users.json*

```json
[  
 {  
  "additionalData": {},  
  "displayName": "User1",  
  "id": "xxx",  
  },
  {  
  "additionalData": {},  
  "displayName": "User2",  
  "id": "yyy",  
  }
]
```

**Get resources**:

```
get resource authenticate users.json output.json
```

*output.json*

```json
{  
 "xxx": [  
  {  
   "additionalData": {},  
   "createdDateTime": "2020-10-28T20:47:31Z",  
   "id": "aaa",  
   "odataType": "#microsoft.graph.passwordAuthenticationMethod"  
  },
  {  
	"additionalData": {},  
	"emailAddress": "foo@bar.com",  
	"id": "bbb",  
	"odataType": "#microsoft.graph.emailAuthenticationMethod"  
  }
 ],
 "yyy": [  
  {  
   "additionalData": {},  
   "createdDateTime": "2023-08-06T17:13:18Z",  
   "id": "ccc",  
   "odataType": "#microsoft.graph.passwordAuthenticationMethod"  
  }  
 ]
}
```

## References

1. **Microsoft Graph API**: 
	- [Official Documentation](https://docs.microsoft.com/en-us/graph/overview)
	- [GitHub Repository](https://github.com/microsoftgraph/msgraph-sdk-go)
2. **Interactive Shell Library (ishell)**: 
	- [GitHub Repository](https://github.com/abiosoft/ishell)
