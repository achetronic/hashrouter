# hashrouter

![GitHub go.mod Go version (subdirectory of monorepo)](https://img.shields.io/github/go-mod/go-version/achetronic/hashrouter)
![GitHub](https://img.shields.io/github/license/achetronic/hashrouter)

![YouTube Channel Subscribers](https://img.shields.io/youtube/channel/subscribers/UCeSb3yfsPNNVr13YsYNvCAw?label=achetronic&link=http%3A%2F%2Fyoutube.com%2Fachetronic)
![X (formerly Twitter) Follow](https://img.shields.io/twitter/follow/achetronic?style=flat&logo=twitter&link=https%3A%2F%2Ftwitter.com%2Fachetronic)

A zero-dependencies HTTP proxy that truly routes requests hash-consistently

## Motivation

This project was created to address a common issue with older proxies that use consistent hashing for request routing. These proxies often fail to maintain real consistency, which can lead to unexpected changes in the backend and increased costs, especially in storage-intensive systems like Varnish.

Additionally, implementing this solution as a plugin for Nginx or Envoy is challenging because their APIs do not allow flexible changes in routing. Therefore, we developed an independent proxy that uses a consistent hash which only updates when backends actually change, ensuring a more stable request distribution and reducing costs.

## Flags

As every configuration parameter can be defined in the config file, there are only few flags that can be defined.
They are described in the following table:

| Name              | Description                    |      Default      | Example                      |
|:------------------|:-------------------------------|:-----------------:|:-----------------------------|
| `--config`        | Path to the YAML config file   | `hashrouter.yaml` | `--config ./hashrouter.yaml` |
| `--log-level`     | Verbosity level for logs       |      `info`       | `--log-level info`           |
| `--disable-trace` | Disable showing traces in logs |      `false`      | `--disable-trace`            |

> Output is thrown always in JSON as it is more suitable for automations

```console
hashrouter run \
    --log-level=info
    --config="./hashrouter.yaml"
```

## Examples

Here you have a complete example. More up-to-date one will always be maintained in 
`docs/prototypes` directory [here](./docs/prototypes)


```yaml

logs:
  show_access_logs: true
  access_logs_fields:
  - ${REQUEST:method}
  - ${REQUEST:host}
  - ${REQUEST:path}
  - ${REQUEST:proto}
  - ${REQUEST:referer}

  - ${REQUEST_HEADER:user-agent}
  - ${REQUEST_HEADER:x-forwarded-for}
  - ${REQUEST_HEADER:x-real-ip}

  - ${RESPONSE_HEADER:content-length}
  - ${RESPONSE_HEADER:content-type}

  - ${EXTRA:request-id}
  - ${EXTRA:hashkey}
  - ${EXTRA:backend}
  

proxies:
  - name: varnish

    listener:
      port: 8080
      address: 0.0.0.0

    backends:
      synchronization: 10s
      static:
        - name: varnish-01
          host: 127.0.0.1:8081
          # (Optional) Healthcheck configuration
          # healthcheck:
          #   timeout: 1s
          #   retries: 3
          #   path: /health

        - name: varnish-02
          host: 127.0.0.1:8082
          # (Optional) Healthcheck configuration 
          # healthcheck:
          #   timeout: 1s
          #   retries: 3
          #   path: /health

        - name: varnish-03
          host: 127.0.0.1:8083

      dns:
        name: varnish-service
        domain: example.com
        port: 80
        # (Optional) Healthcheck configuration 
        # healthcheck:
        #   timeout: 1s
        #   retries: 3
        #   path: /health

    hash_key:

      # Key to generate a hash from the request. It can be composed using any of the following:
      # ${REQUEST:scheme}, ${REQUEST:host}, ${REQUEST:port}, ${REQUEST:path}, ${REQUEST:query}
      # ${REQUEST:method}, ${REQUEST:proto}
      pattern: "${REQUEST:path}"

    # Aditional options such as hashing mode or TTL
    options:
      protocol: http

      # protocol: http2
      # certificate: /etc/ssl/certs/achetronic.pem
      # key: /etc/ssl/private/achetronic.key

      # (optional) Timeout in milliseconds to close connection to the backend
      backend_connect_timeout_ms: 40

      # (optional) Hashring always assigns the same backend to the hashkey.
      # If the backend is down, you can try another backend until exaushting all of them
      # by enabling this option
      try_another_backend_on_failure: true

```

> ATTENTION:
> If you detect some mistake on the config, open an issue to fix it. This way we all will benefit

## How to deploy

This project is designed specially for Kubernetes, but also provides binary files 
and Docker images to make it easy to be deployed however wanted

### Binaries

Binary files for most popular platforms will be added to the [releases](https://github.com/achetronic/hashrouter/releases)

### Kubernetes

You can deploy `hashrouter` in Kubernetes using Helm as follows:

```console
helm repo add hashrouter https://achetronic.github.io/hashrouter/

helm upgrade --install --wait hashrouter \
  --namespace hashrouter \
  --create-namespace achetronic/hashrouter
```

> More information and Helm packages [here](https://achetronic.github.io/hashrouter/)


### Docker

Docker images can be found in GitHub's [packages](https://github.com/achetronic/hashrouter/pkgs/container/hashrouter) 
related to this repository

> Do you need it in a different container registry? I think this is not needed, but if I'm wrong, please, let's discuss 
> it in the best place for that: an issue

## How to contribute

We are open to external collaborations for this project: improvements, bugfixes, whatever.

For doing it, open an issue to discuss the need of the changes, then:

- Fork the repository
- Make your changes to the code
- Open a PR and wait for review

The code will be reviewed and tested (always)

> We are developers and hate bad code. For that reason we ask you the highest quality
> on each line of code to improve this project on each iteration.

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
