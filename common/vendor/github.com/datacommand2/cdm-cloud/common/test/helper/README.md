# Unit Test Helper
단위 테스트를 위한 helper 패키지로서, 테스트를 위한 사전준비 및 종료 기능과 기타 공통 기능을 제공한다.

---

## 환경
### Local
로컬 환경에서 Backing Service 들을 올리기 위한 docker swarm compose 는 다음과 같다.
```yaml
version: "3.8"

services:
  etcd:
    image: registry.datacommand.co.kr/cdm-cloud-etcd:HEAD
    environment:
      - CDM_SERVICE_NAME=cdm-cloud-etcd
      - CDM_SERVICE_ADVERTISE_PORT=2379
      - ETCD_ID={{.Task.Slot}}
      - ETCD_USER=cdm
      - ETCD_PASSWORD=password

  rabbitmq:
    image: registry.datacommand.co.kr/cdm-cloud-rabbitmq:HEAD
    environment:
      - CDM_SERVICE_NAME=cdm-cloud-rabbitmq
      - CDM_SERVICE_ADVERTISE_PORT=5672
      - RABBITMQ_NODENAME=rabbit@tasks.rabbitmq
      - RABBITMQ_ERLANG_COOKIE=cdmcookie
      - RABBITMQ_DEFAULT_USER=cdm
      - RABBITMQ_DEFAULT_PASS=password
      - RABBITMQ_USE_LONGNAME=true
      - RABBITMQ_ROLE=LEADER
            
  cockroach:
    image: registry.datacommand.co.kr/cdm-cloud-cockroach:HEAD
    hostname: tasks.cockroach
    environment:
      - CDM_SERVICE_NAME=cdm-cloud-cockroach
      - CDM_SERVICE_ADVERTISE_PORT=26257
      - CDM_SERVICE_METADATA=dialect=postgres
      - COCKROACH_ROLE=LEADER
      - COCKROACH_LEADER_NODENAME=tasks.cockroach
      - COCKROACH_INSECURE=true
      - COCKROACH_DEFAULT_USER=cdm
      - COCKROACH_DEFAULT_PASS=password
      - COCKROACH_DEFAULT_DATABASE=cdm

networks:
  default:
    driver: weaveworks/net-plugin:2.6.5
    attachable: true
```

### Gitlab Runner
GitLab Runner 에서 Backing Service 들을 올리기 위한 CI 는 다음과 같다.
```yaml
services:
  - name: registry.datacommand.co.kr/cdm-cloud-etcd:HEAD
    alias: "etcd"
  - name: registry.datacommand.co.kr/cdm-cloud-rabbitmq:HEAD
    alias: "rabbitmq"
    command: ["rabbitmq-server"]
  - name: registry.datacommand.co.kr/cdm-cloud-cockroach:HEAD
    alias: "cockroach"

variables:
  ETCD_ID: "1"
  ETCD_USER: "cdm"
  ETCD_PASSWORD: "password"

  RABBITMQ_DEFAULT_USER: "cdm"
  RABBITMQ_DEFAULT_PASS: "password"

  COCKROACH_ROLE: "LEADER"
  COCKROACH_LEADER_NODENAME: "localhost"
  COCKROACH_DEFAULT_DATABASE: "cdm"
  COCKROACH_DEFAULT_USER: "cdm"
  COCKROACH_DEFAULT_PASS: "password"
  COCKROACH_INSECURE: "true"
```

---

## 초기화 및 종료
`TestMain` 함수에서 단위 테스트를 시작하기 전과 완료된 후에 `Init()`, `Close()` 를 호출해줄 수 있다.
`Init()` 함수는 **CDM 서비스들**이 사용하는 Backing Service(broker, store, database) 들을 discover 혹은 lookup 해서 Service Registry 에 register 하고,
데이터베이스 스키마를 생성, 초기데이터를 적재하는 기능을 수행한다. 추가로 테스트 중 발생하는 데이터베이스의 변경분은 모두 롤백처리한다. 
```go
func TestMain(m *testing.M) {
    if err := helper.Init(); err != nil {
        panic(err)
    } else {
        defer helper.Close()
    }

    _ = m.Run()
}
```

`Init()` 함수는 옵션을 지정하여 호출할 수 있으며, 아무 옵션도 지정하지 않을 경우 기본값으로 초기화한다. 지정할 수 있는 옵션은 다음과 같다.
```go
// Broker 는 단위 테스트의 브로커를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func Broker(registerName, discoverName, host string, port int, metadata map[string]string, username, password string) Option

// Store 는 단위 테스트의 키저장소를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func Store(registerName, discoverName, host string, port int, metadata map[string]string, username, password string) Option

// Database 는 단위 테스트의 데이터베이스를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func Database(registerName, discoverName, host string, port int, metadata map[string]string, dbName, username, password string) Option

// DatabaseDDLScriptURI 는 단위 테스트의 데이터베이스 스키마생성 스크립트 URI 를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func DatabaseDDLScriptURI(uri string) Option

// DatabaseDMLScriptURI 는 단위 테스트의 데이터베이스 초기 적재 스크립트 URI 를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func DatabaseDMLScriptURI(uri string) Option
```

**기본값**:

| Key                     | Value                                                                               |
|-------------------------|-------------------------------------------------------------------------------------|
| BrokerRegisterName      | "broker_test_normal"                                                                |
| BrokerServiceName       | "cdm-cloud-rabbitmq"                                                                |
| BrokerServiceHost       | "rabbitmq"                                                                          |
| BrokerServicePort       | 5672                                                                                |
| BrokerServiceMetadata   | map[string]string{}                                                                 |
| BrokerAuthUsername      | "cdm"                                                                               |
| BrokerAuthPassword      | "password"                                                                          |
| StoreRegisterName       | "store_test_normal"                                                                 |
| StoreServiceName        | "cdm-cloud-etcd"                                                                    |
| StoreServiceHost        | "etcd"                                                                              |
| StoreServicePort        | 2379                                                                                |
| StoreServiceMetadata    | map[string]string{}                                                                 |
| StoreAuthUsername       | "cdm"                                                                               |
| StoreAuthPassword       | "password"                                                                          |
| DatabaseRegisterName    | "database_test_normal"                                                              |
| DatabaseServiceName     | "cdm-cloud-cockroach"                                                               |
| DatabaseServiceHost     | "cockroach"                                                                         |
| DatabaseServicePort     | 26257                                                                               |
| DatabaseServiceMetadata | map[string]string{"dialect": "postgres"}                                            |
| DatabaseDBName          | "cdm"                                                                               |
| DatabaseAuthUsername    | "cdm"                                                                               |
| DatabaseAuthPassword    | "password"                                                                          |
| DatabaseDDLScriptURI    | "http://github.com/datacommand2/cdm-cloud/documents/-/raw/master/database/cdm-cloud-ddl.sql" |
| DatabaseDMLScriptURI    | "http://github.com/datacommand2/cdm-cloud/documents/-/raw/master/database/cdm-cloud-dml.sql" |
