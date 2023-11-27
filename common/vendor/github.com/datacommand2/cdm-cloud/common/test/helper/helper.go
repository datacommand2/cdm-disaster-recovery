package helper

import (
	"github.com/datacommand2/cdm-cloud/common/broker"
	"github.com/datacommand2/cdm-cloud/common/database"
	"github.com/datacommand2/cdm-cloud/common/logger"
	"github.com/datacommand2/cdm-cloud/common/store"
	"github.com/datacommand2/cdm-cloud/common/sync"
	"github.com/datacommand2/cdm-cloud/common/test"
	microLogger "github.com/micro/go-micro/v2/logger"
	"io/ioutil"
	"net/http"
	"time"
)

var (
	defaultBrokerRegisterName    = "broker_test_normal"
	defaultBrokerServiceName     = "cdm-cloud-rabbitmq"
	defaultBrokerServiceHost     = "rabbitmq"
	defaultBrokerServicePort     = 5672
	defaultBrokerServiceMetadata = map[string]string{}
	defaultBrokerAuthUsername    = "cdm"
	defaultBrokerAuthPassword    = "password"

	defaultStoreRegisterName    = "store_test_normal"
	defaultStoreServiceName     = "cdm-cloud-etcd"
	defaultStoreServiceHost     = "etcd"
	defaultStoreServicePort     = 2379
	defaultStoreServiceMetadata = map[string]string{}
	defaultStoreAuthUsername    = "cdm"
	defaultStoreAuthPassword    = "password"

	defaultDatabaseRegisterName    = "database_test_normal"
	defaultDatabaseServiceName     = "cdm-cloud-cockroach"
	defaultDatabaseServiceHost     = "cockroach"
	defaultDatabaseServicePort     = 26257
	defaultDatabaseServiceMetadata = map[string]string{"dialect": "postgres"}
	defaultDatabaseDBName          = "cdm"
	defaultDatabaseAuthUsername    = "cdm"
	defaultDatabaseAuthPassword    = "password"
	defaultDatabaseDDLScriptURI    = "http://github.com/datacommand2/cdm-cloud/documents/-/raw/1-0-stable/database/cdm-cloud-ddl.sql"
	defaultDatabaseDMLScriptURI    = "http://github.com/datacommand2/cdm-cloud/documents/-/raw/1-0-stable/database/cdm-cloud-dml.sql"

	defaultSyncRegisterName    = "sync_test_normal"
	defaultSyncServiceName     = "cdm-cloud-etcd"
	defaultSyncServiceHost     = "etcd"
	defaultSyncServicePort     = 2379
	defaultSyncServiceMetadata = map[string]string{}
	defaultSyncAuthUsername    = "cdm"
	defaultSyncAuthPassword    = "password"
)

// Init 은 단위 테스트를 위한 Backing Service Register 및 Initialize 를 위한 함수이다.
func Init(opts ...Option) (err error) {
	var options = Options{
		BrokerRegisterName:    defaultBrokerRegisterName,
		BrokerServiceName:     defaultBrokerServiceName,
		BrokerServiceHost:     defaultBrokerServiceHost,
		BrokerServicePort:     defaultBrokerServicePort,
		BrokerServiceMetadata: defaultBrokerServiceMetadata,
		BrokerAuthUsername:    defaultBrokerAuthUsername,
		BrokerAuthPassword:    defaultBrokerAuthPassword,

		StoreRegisterName:    defaultStoreRegisterName,
		StoreServiceName:     defaultStoreServiceName,
		StoreServiceHost:     defaultStoreServiceHost,
		StoreServicePort:     defaultStoreServicePort,
		StoreServiceMetadata: defaultStoreServiceMetadata,
		StoreAuthUsername:    defaultStoreAuthUsername,
		StoreAuthPassword:    defaultStoreAuthPassword,

		DatabaseRegisterName:    defaultDatabaseRegisterName,
		DatabaseServiceName:     defaultDatabaseServiceName,
		DatabaseServiceHost:     defaultDatabaseServiceHost,
		DatabaseServicePort:     defaultDatabaseServicePort,
		DatabaseServiceMetadata: defaultDatabaseServiceMetadata,
		DatabaseDBName:          defaultDatabaseDBName,
		DatabaseAuthUsername:    defaultDatabaseAuthUsername,
		DatabaseAuthPassword:    defaultDatabaseAuthPassword,
		DatabaseDDLScriptURI:    []string{defaultDatabaseDDLScriptURI},
		DatabaseDMLScriptURI:    []string{defaultDatabaseDMLScriptURI},

		SyncRegisterName:    defaultSyncRegisterName,
		SyncServiceName:     defaultSyncServiceName,
		SyncServiceHost:     defaultSyncServiceHost,
		SyncServicePort:     defaultSyncServicePort,
		SyncServiceMetadata: defaultSyncServiceMetadata,
		SyncAuthUsername:    defaultSyncAuthUsername,
		SyncAuthPassword:    defaultSyncAuthPassword,
	}

	for _, o := range opts {
		o(&options)
	}

	// init logger
	_ = logger.Init(microLogger.WithLevel(microLogger.TraceLevel))
	microLogger.DefaultLogger = microLogger.NewHelper(logger.NewLogger(microLogger.WithLevel(microLogger.TraceLevel)))

	// register backing services
	if err := registerServices(options); err != nil {
		return err
	}

	// initialize database schema
	if err := initDatabase(options); err != nil {
		return err
	}

	// initialize backing services
	if err := initServices(options); err != nil {
		return err
	}

	return nil
}

func registerServices(opts Options) error {
	// register broker service
	if err := test.RegisterService(
		opts.BrokerRegisterName,
		opts.BrokerServiceName,
		opts.BrokerServiceHost,
		opts.BrokerServicePort,
		opts.BrokerServiceMetadata,
	); err != nil {
		return err
	}

	// register store service
	if err := test.RegisterService(
		opts.StoreRegisterName,
		opts.StoreServiceName,
		opts.StoreServiceHost,
		opts.StoreServicePort,
		opts.StoreServiceMetadata,
	); err != nil {
		return err
	}

	// register sync back end
	if err := test.RegisterService(
		opts.SyncRegisterName,
		opts.SyncServiceName,
		opts.SyncServiceHost,
		opts.SyncServicePort,
		opts.SyncServiceMetadata,
	); err != nil {
		return err
	}

	// register database service
	if err := test.RegisterService(
		opts.DatabaseRegisterName,
		opts.DatabaseServiceName,
		opts.DatabaseServiceHost,
		opts.DatabaseServicePort,
		opts.DatabaseServiceMetadata,
	); err != nil {
		return err
	}

	// wait for register services
	time.Sleep(5 * time.Second)

	return nil
}

func initServices(opts Options) error {
	// init broker
	if err := broker.Init(
		opts.BrokerRegisterName,
		opts.BrokerAuthUsername,
		opts.BrokerAuthPassword,
	); err != nil {
		return err
	}

	// init database connection info
	database.Init(
		opts.DatabaseRegisterName,
		opts.DatabaseDBName,
		opts.DatabaseAuthUsername,
		opts.DatabaseAuthPassword,
		database.SSLEnable(false),
		database.TestMode(),
	)

	// connect broker
	if err := broker.Connect(); err != nil {
		return err
	}

	// init store connection info
	store.Init(
		opts.StoreRegisterName,
		store.Auth(opts.StoreAuthUsername, opts.StoreAuthPassword),
	)
	// connect store
	err := store.Connect()
	if err != nil {
		_ = broker.Disconnect()
		return err
	}

	// connect database
	if err := database.Connect(); err != nil {
		_ = broker.Disconnect()
		_ = store.Close()
		return err
	}

	err = sync.Init(opts.SyncRegisterName, sync.Auth(opts.SyncAuthUsername, opts.SyncAuthPassword))
	if err != nil {
		_ = broker.Disconnect()
		_ = store.Close()
		_ = database.Close()
		return err
	}
	return nil
}

func getURIBody(uri string) ([]byte, error) {
	rsp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rsp.Body.Close()
	}()

	return ioutil.ReadAll(rsp.Body)
}

func initDatabase(options Options) error {
	db, err := database.Open(
		options.DatabaseRegisterName,
		options.DatabaseDBName,
		options.DatabaseAuthUsername,
		options.DatabaseAuthPassword,
		database.SSLEnable(false),
	)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	// execute database scripts
	for _, uri := range append(options.DatabaseDDLScriptURI, options.DatabaseDMLScriptURI...) {
		script, err := getURIBody(uri)
		if err != nil {
			return err
		}

		if err := db.Exec(string(script)).Error; err != nil {
			return err
		}
	}

	return nil
}

// Close 는 단위 테스트를 위해 사용한 자원을 해제하기 위한 함수이다.
func Close() {
	_ = database.Close()
	_ = store.Close()
	_ = broker.Disconnect()
	_ = sync.Close()
}
