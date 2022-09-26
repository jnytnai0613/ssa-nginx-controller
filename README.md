# ssa-nginx-controller
## Description
A controller that automates Nginx deployment, operations, and connection settings.

## Automation of NGINX server operations
Automate the following operations
- Creation of the following kuberndtes resources
   - ConfigMap: default.conf and index.html with data
   - Deployment: NGINX server
   - Service: Routing to NGINX
   - Ingress: external access to the Service
   - Secret: Data contains CA certificate, server certificate and private key required for SSL termination of Ingress
   - Secret: Client certificate and private key required for access to Ingress in data
- Change Resource Name
- Remove old Resource after renaming
- Change resource definition
- Automatic reload when default.conf is changed (monitored by inotifywait)

**NOTE:** Currently, renaming and field modification of each resource is supported, but modification of the Ingress host field when ingressSecureEnabled = true is not supported.

## yaml example
```yaml
apiVersion: ssanginx.jnytnai0613.github.io/v1
kind: SSANginx
metadata:
  name: ssanginx-sample
  namespace: ssa-nginx-controller-system
spec:
  deploymentName: nginx
  deploymentSpec:
    replicas: 3
    strategy:
      type: RollingUpdate
      rollingUpdate:
        maxSurge: 30%
        maxUnavailable: 30%
    template:
      spec:
        containers:
          - name: nginx
            image: nginx:latest
  configMapName: nginx
  configMapData:
    default.conf: |
      server {
            listen 80 default_server;
            listen [::]:80 default_server ipv6only=on;
            root /usr/share/nginx/html;
            index index.html index.htm mod-index.html;
          server_name localhost;
      }
    mod-index.html: |
      <!DOCTYPE html>
      <html>
      <head>
      <title>Yeahhhhhhh!! Welcome to nginx!!</title>
      <style>
      html { color-scheme: light dark; }
      body { width: 35em; margin: 0 auto;
      font-family: Tahoma, Verdana, Arial, sans-serif; }
      </style>
      </head>
      <body>
      <h1>Yeahhhhhhh!! Welcome to nginx!!</h1>
      <p>If you see this page, the nginx web server is successfully installed and
      working. Further configuration is required.</p>
      <p>For online documentation and support please refer to
      <a href="http://nginx.org/">nginx.org</a>.<br/>
      Commercial support is available at
      <a href="http://nginx.com/">nginx.com</a>.</p>
      <p><em>Thank you for using nginx.</em></p>
      </body>
      </html>
  serviceName: nginx
  serviceSpec:
    type: ClusterIP
    ports:
    - protocol: TCP
      port: 80
      targetPort: 80
  ingressName: nginx
  ingressSpec:
    rules:
    - host: nginx.example.com
      http:
        paths:
        - path: /
          pathType: Prefix
          backend:
            service:
              name: nginx
              port:
                number: 80
  ingressSecureEnabled: true
```

## Description to each field of CR
The CR yaml file is located in the config/samples directory.

### .spec.deploymentSpec
| Name       | Type               | Required      |
| ---------- | ------------------ | ------------- |
| replicas   | int32              | false         |
| strategy   | DeploymentStrategy | false         |

Other fields cannot be specified.　  
Check the following reference for a description of the strategy field.  
https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/deployment-v1/#DeploymentSpec


### .spec.deploymentSpec.template.spec.conatiners
| Name       | Type               | Required      |
| ---------- | ------------------ | ------------- |
| name       | string             | false         |
| image      | string             | true          |

The other fields are options.See the following reference for possible fields.  
https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#PodSpec

### .spec.configmapName
| Name           | Type               | Required      |
| -------------- | ------------------ | ------------- |
| configmapName  | string             | true          |

### .spec.configMapData
| Name           | Type               | Required      |
| -------------- | ------------------ | ------------- |
| default.conf   | map[string]string  | true          |
| index.html     | map[string]string  | true          |

index.hmtl is mod-index.html by default. The name can be changed.  
However, renaming of default.conf is not supported.

### .spec.serviceName
| Name           | Type               | Required      |
| -------------- | ------------------ | ------------- |
| serviceName    | string             | true          |

### .spec.serviceSpec
The serviceSpec field is required.  
However, selectors are automatically assigned by the controller and are not required.  
Check the following reference for a description of the serviceSpec field.  
https://kubernetes.io/docs/reference/kubernetes-api/service-resources/service-v1/

### .spec.ingressName
| Name           | Type               | Required      |
| -------------- | ------------------ | ------------- |
| ingressName    | string             | true          |

### .spec.ingressSpec
The ingressSpec field is required. 
However, if TLS settings are to be made, the field does not need to be added, as it will be set automatically by setting ingressSecureEnabled to true, as described below.  
Check the following reference for a description of the ingressSpec field.  
https://kubernetes.io/docs/reference/kubernetes-api/service-resources/ingress-v1/

### .spec.ingressSecureEnabled
| Name                 | Type               | Required      |
| -------------------- | ------------------ | ------------- |
| ingressSecureEnabled | bool               | true          |

By setting ingressSecureEnabled to true, the following fields are automatically added to the Ingress resource. Also, the Secret resource ca-secret is automatically created, containing the CA certificate, the server certificate, and the private key for the server certificate.  
```yaml
tls:
- hosts:
  - test-nginx.example.com
  secretName: ca-secret
````

## SSL Termination for Ingress
The following Secret is automatically created by setting the .spec.ingressSecureEnabled field in CustomResource to true.
```
NAME                         TYPE                DATA   AGE
secret/ca-secret             Opaque              3      32m
secret/cli-secret            Opaque              2      32m
```
TLS settings are also automatically added to Ingress.
- Add the following annotations to enable client authentication.
Each annotation is explained below.
https://github.com/kubernetes/ingress-nginx/blob/main/docs/user-guide/nginx-configuration/annotations.md#client-certificate-authentication
```
$ kubectl -n ssa-nginx-controller-system get ingress nginx -ojson | jq '.metadata.annotations'
{
  "nginx.ingress.kubernetes.io/auth-tls-secret": "ssa-nginx-controller-system/ca-secret",
  "nginx.ingress.kubernetes.io/auth-tls-verify-client": "on",
  "nginx.ingress.kubernetes.io/rewrite-target": "/"
}
```
- Add the .spec.tls field.
Secret ca-secret is automatically created by the Controller and is automatically specified.
Also, hosts will automatically use the value specified in CustomResource's '.spec.ingressSpec.rules[].host'.
```
$ kubectl  -n ssa-nginx-controller-system get ingress nginx -ojson | jq '.spec.tls'
[
  {
    "hosts": [
      "nginx.example.com"
    ],
    "secretName": "ca-secret"
  }
]
```
### Connection using Ingress
First, download the client certificate and private key from Secret cli-secret.
```
$ kubectl -n ssa-nginx-controller-system get secrets cli-secret -ojsonpath='{.data.client\.crt}' | base64 -d > client.crt

$ kubectl -n ssa-nginx-controller-system get secrets cli-secret -ojsonpath='{.data.client\.key}' | base64 -d > client.key
```
It can then be accessed with the following command
```
curl --key client.key --cert client.crt https://nginx.example.com:443/ --resolve nginx.example.com:443:<IP Address> -kv
```
The various certificates and private keys are generated by calling the functions in the following go file from the Controller.
https://github.com/jnytnai0613/ssa-nginx-controller/blob/main/pkg/pki/certificate.go

Since the -v option is given to curl, the CN specified in certificate.go can be confirmed.
```
*  subject: C=JP; O=Example Org; OU=Example Org Unit; CN=server
*  start date: Sep 26 03:21:24 2022 GMT
*  expire date: Dec 31 00:00:00 2031 GMT
*  issuer: C=JP; O=Example Org; OU=Example Org Unit; CN=ca
```

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.  
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Build and push your image to the location specified by `IMG`:
	
```sh
make docker-build docker-push IMG=<some-registry>/ssa-nginx-controller:tag
```
	
2. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/ssa-nginx-controller:tag
```

3. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

4. See our deployment resources

```sh
kubectl -n ssa-nginx-controller-system get deployment,pod
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
