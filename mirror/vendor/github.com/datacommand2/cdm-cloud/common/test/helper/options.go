package helper

// Option 는 Options 를 인자로 받는 함수의 타입이다.
type Option func(*Options)

// Options 는 단위 테스트 초기화 옵션에 대한 구조체이다.
type Options struct {
	BrokerRegisterName    string
	BrokerServiceName     string
	BrokerServiceHost     string
	BrokerServicePort     int
	BrokerServiceMetadata map[string]string
	BrokerAuthUsername    string
	BrokerAuthPassword    string

	StoreRegisterName    string
	StoreServiceName     string
	StoreServiceHost     string
	StoreServicePort     int
	StoreServiceMetadata map[string]string
	StoreAuthUsername    string
	StoreAuthPassword    string

	DatabaseRegisterName    string
	DatabaseServiceName     string
	DatabaseServiceHost     string
	DatabaseServicePort     int
	DatabaseServiceMetadata map[string]string
	DatabaseDBName          string
	DatabaseAuthUsername    string
	DatabaseAuthPassword    string
	DatabaseDDLScriptURI    []string
	DatabaseDMLScriptURI    []string

	SyncRegisterName    string
	SyncServiceName     string
	SyncServiceHost     string
	SyncServicePort     int
	SyncServiceMetadata map[string]string
	SyncAuthUsername    string
	SyncAuthPassword    string
}

// Broker 는 단위 테스트의 브로커를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func Broker(registerName, discoverName, host string, port int, metadata map[string]string, username, password string) Option {
	return func(o *Options) {
		o.BrokerRegisterName = registerName
		o.BrokerServiceName = discoverName
		o.BrokerServiceHost = host
		o.BrokerServicePort = port
		o.BrokerServiceMetadata = metadata
		o.BrokerAuthUsername = username
		o.BrokerAuthPassword = password
	}
}

// Store 는 단위 테스트의 키저장소를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func Store(registerName, discoverName, host string, port int, metadata map[string]string, username, password string) Option {
	return func(o *Options) {
		o.StoreRegisterName = registerName
		o.StoreServiceName = discoverName
		o.StoreServiceHost = host
		o.StoreServicePort = port
		o.StoreServiceMetadata = metadata
		o.StoreAuthUsername = username
		o.StoreAuthPassword = password
	}
}

// Sync 는 단위 테스트의 sync backend를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func Sync(registerName, discoverName, host string, port int, metadata map[string]string, username, password string) Option {
	return func(o *Options) {
		o.SyncRegisterName = registerName
		o.SyncServiceName = discoverName
		o.SyncServiceHost = host
		o.SyncServicePort = port
		o.SyncServiceMetadata = metadata
		o.SyncAuthUsername = username
		o.SyncAuthPassword = password
	}
}

// Database 는 단위 테스트의 데이터베이스를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func Database(registerName, discoverName, host string, port int, metadata map[string]string, dbName, username, password string) Option {
	return func(o *Options) {
		o.DatabaseRegisterName = registerName
		o.DatabaseServiceName = discoverName
		o.DatabaseServiceHost = host
		o.DatabaseServicePort = port
		o.DatabaseServiceMetadata = metadata
		o.DatabaseDBName = dbName
		o.DatabaseAuthUsername = username
		o.DatabaseAuthPassword = password
	}
}

// DatabaseDDLScriptURI 는 단위 테스트의 데이터베이스 스키마생성 스크립트 URI 를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func DatabaseDDLScriptURI(uri string) Option {
	return func(o *Options) {
		o.DatabaseDDLScriptURI = append(o.DatabaseDDLScriptURI, uri)
	}
}

// DatabaseDMLScriptURI 는 단위 테스트의 데이터베이스 초기 적재 스크립트 URI 를 기본값이 아닌 다른 값으로 초기화하기 위한 옵션 함수이다.
func DatabaseDMLScriptURI(uri string) Option {
	return func(o *Options) {
		o.DatabaseDMLScriptURI = append(o.DatabaseDMLScriptURI, uri)
	}
}
