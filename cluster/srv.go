// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017-2021 SIGNAL18 CLOUD SAS
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.
// Redistribution/Reuse of this code is permitted under the GNU v3 license, as
// an additional term, ALL code must carry the original Author(s) credit in comment form.
// See LICENSE in this directory for the integral text.

package cluster

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc64"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"

	"github.com/go-sql-driver/mysql"
	"github.com/hpcloud/tail"
	"github.com/jmoiron/sqlx"
	"github.com/signal18/replication-manager/utils/dbhelper"
	"github.com/signal18/replication-manager/utils/gtid"
	"github.com/signal18/replication-manager/utils/misc"
	"github.com/signal18/replication-manager/utils/s18log"
	"github.com/signal18/replication-manager/utils/state"
)

// ServerMonitor defines a server to monitor.
type ServerMonitor struct {
	Id                          string                       `json:"id"` //Unique name given by cluster & crc64(URL) used by test to provision
	Name                        string                       `json:"name"`
	Domain                      string                       `json:"domain"`
	ServiceName                 string                       `json:"serviceName"`
	Conn                        *sqlx.DB                     `json:"-"`
	User                        string                       `json:"user"`
	Pass                        string                       `json:"-"`
	URL                         string                       `json:"url"`
	DSN                         string                       `json:"dsn"`
	Host                        string                       `json:"host"`
	Port                        string                       `json:"port"`
	TunnelPort                  string                       `json:"tunnelPort"`
	IP                          string                       `json:"ip"`
	Strict                      string                       `json:"strict"`
	ServerID                    uint64                       `json:"serverId"`
	GTIDBinlogPos               *gtid.List                   `json:"gtidBinlogPos"`
	CurrentGtid                 *gtid.List                   `json:"currentGtid"`
	SlaveGtid                   *gtid.List                   `json:"slaveGtid"`
	IOGtid                      *gtid.List                   `json:"ioGtid"`
	FailoverIOGtid              *gtid.List                   `json:"failoverIoGtid"`
	GTIDExecuted                string                       `json:"gtidExecuted"`
	ReadOnly                    string                       `json:"readOnly"`
	State                       string                       `json:"state"`
	PrevState                   string                       `json:"prevState"`
	FailCount                   int                          `json:"failCount"`
	FailSuspectHeartbeat        int64                        `json:"failSuspectHeartbeat"`
	ClusterGroup                *Cluster                     `json:"-"` //avoid recusive json
	BinaryLogFile               string                       `json:"binaryLogFile"`
	BinaryLogFilePrevious       string                       `json:"binaryLogFilePrevious"`
	BinaryLogPos                string                       `json:"binaryLogPos"`
	FailoverMasterLogFile       string                       `json:"failoverMasterLogFile"`
	FailoverMasterLogPos        string                       `json:"failoverMasterLogPos"`
	FailoverSemiSyncSlaveStatus bool                         `json:"failoverSemiSyncSlaveStatus"`
	Process                     *os.Process                  `json:"process"`
	SemiSyncMasterStatus        bool                         `json:"semiSyncMasterStatus"`
	SemiSyncSlaveStatus         bool                         `json:"semiSyncSlaveStatus"`
	RplMasterStatus             bool                         `json:"rplMasterStatus"`
	HaveEventScheduler          bool                         `json:"eventScheduler"`
	HaveSemiSync                bool                         `json:"haveSemiSync"`
	HaveInnodbTrxCommit         bool                         `json:"haveInnodbTrxCommit"`
	HaveChecksum                bool                         `json:"haveInnodbChecksum"`
	HaveLogGeneral              bool                         `json:"haveLogGeneral"`
	HaveBinlog                  bool                         `json:"haveBinlog"`
	HaveBinlogSync              bool                         `json:"haveBinLogSync"`
	HaveBinlogRow               bool                         `json:"haveBinlogRow"`
	HaveBinlogAnnotate          bool                         `json:"haveBinlogAnnotate"`
	HaveBinlogSlowqueries       bool                         `json:"haveBinlogSlowqueries"`
	HaveBinlogCompress          bool                         `json:"haveBinlogCompress"`
	HaveBinlogSlaveUpdates      bool                         `json:"HaveBinlogSlaveUpdates"`
	HaveGtidStrictMode          bool                         `json:"haveGtidStrictMode"`
	HaveMySQLGTID               bool                         `json:"haveMysqlGtid"`
	HaveMariaDBGTID             bool                         `json:"haveMariadbGtid"`
	HaveSlowQueryLog            bool                         `json:"haveSlowQueryLog"`
	HavePFSSlowQueryLog         bool                         `json:"havePFSSlowQueryLog"`
	HaveMetaDataLocksLog        bool                         `json:"haveMetaDataLocksLog"`
	HaveQueryResponseTimeLog    bool                         `json:"haveQueryResponseTimeLog"`
	HaveDiskMonitor             bool                         `json:"haveDiskMonitor"`
	HaveSQLErrorLog             bool                         `json:"haveSQLErrorLog"`
	HavePFS                     bool                         `json:"havePFS"`
	HaveWsrep                   bool                         `json:"haveWsrep"`
	HaveReadOnly                bool                         `json:"haveReadOnly"`
	HaveNoMasterOnStart         bool                         `json:"haveNoMasterOnStart"`
	IsWsrepSync                 bool                         `json:"isWsrepSync"`
	IsWsrepDonor                bool                         `json:"isWsrepDonor"`
	IsWsrepPrimary              bool                         `json:"isWsrepPrimary"`
	IsMaxscale                  bool                         `json:"isMaxscale"`
	IsRelay                     bool                         `json:"isRelay"`
	IsSlave                     bool                         `json:"isSlave"`
	IsVirtualMaster             bool                         `json:"isVirtualMaster"`
	IsMaintenance               bool                         `json:"isMaintenance"`
	IsCompute                   bool                         `json:"isCompute"` //Used to idenfied spider compute nide
	IsDelayed                   bool                         `json:"isDelayed"`
	IsFull                      bool                         `json:"isFull"`
	Ignored                     bool                         `json:"ignored"`
	Prefered                    bool                         `json:"prefered"`
	PreferedBackup              bool                         `json:"preferedBackup"`
	InCaptureMode               bool                         `json:"inCaptureMode"`
	LongQueryTimeSaved          string                       `json:"longQueryTimeSaved"`
	LongQueryTime               string                       `json:"longQueryTime"`
	LogOutput                   string                       `json:"logOutput"`
	SlowQueryLog                string                       `json:"slowQueryLog"`
	SlowQueryCapture            bool                         `json:"slowQueryCapture"`
	BinlogDumpThreads           int                          `json:"binlogDumpThreads"`
	MxsVersion                  int                          `json:"maxscaleVersion"`
	MxsHaveGtid                 bool                         `json:"maxscaleHaveGtid"`
	MxsServerName               string                       `json:"maxscaleServerName"` //Unique server Name in maxscale conf
	MxsServerStatus             string                       `json:"maxscaleServerStatus"`
	ProxysqlHostgroup           string                       `json:"proxysqlHostgroup"`
	RelayLogSize                uint64                       `json:"relayLogSize"`
	Replications                []dbhelper.SlaveStatus       `json:"replications"`
	LastSeenReplications        []dbhelper.SlaveStatus       `json:"lastSeenReplications"`
	MasterStatus                dbhelper.MasterStatus        `json:"masterStatus"`
	SlaveStatus                 *dbhelper.SlaveStatus        `json:"-"`
	ReplicationSourceName       string                       `json:"replicationSourceName"`
	DBVersion                   *dbhelper.MySQLVersion       `json:"dbVersion"`
	Version                     int                          `json:"-"`
	QPS                         int64                        `json:"qps"`
	ReplicationHealth           string                       `json:"replicationHealth"`
	EventStatus                 []dbhelper.Event             `json:"eventStatus"`
	FullProcessList             []dbhelper.Processlist       `json:"-"`
	Variables                   map[string]string            `json:"-"`
	EngineInnoDB                map[string]string            `json:"engineInnodb"`
	ErrorLog                    s18log.HttpLog               `json:"errorLog"`
	SlowLog                     s18log.SlowLog               `json:"-"`
	Status                      map[string]string            `json:"-"`
	PrevStatus                  map[string]string            `json:"-"`
	PFSQueries                  map[string]dbhelper.PFSQuery `json:"-"` //PFS queries
	SlowPFSQueries              map[string]dbhelper.PFSQuery `json:"-"` //PFS queries from slow
	DictTables                  map[string]dbhelper.Table    `json:"-"`
	Tables                      []dbhelper.Table             `json:"-"`
	Disks                       []dbhelper.Disk              `json:"-"`
	Plugins                     map[string]dbhelper.Plugin   `json:"-"`
	Users                       map[string]dbhelper.Grant    `json:"-"`
	MetaDataLocks               []dbhelper.MetaDataLock      `json:"-"`
	ErrorLogTailer              *tail.Tail                   `json:"-"`
	SlowLogTailer               *tail.Tail                   `json:"-"`
	MonitorTime                 int64                        `json:"-"`
	PrevMonitorTime             int64                        `json:"-"`
	maxConn                     string                       `json:"maxConn"` // used to back max connection for failover
	Datadir                     string                       `json:"-"`
	SlapOSDatadir               string                       `json:"slaposDatadir"`
	PostgressDB                 string                       `json:"postgressDB"`
	CrcTable                    *crc64.Table                 `json:"-"`
	TLSConfigUsed               string                       `json:"tlsConfigUsed"` //used to track TLS config during key rotation
	SSTPort                     string                       `json:"sstPort"`       //used to send data to dbjobs
	Agent                       string                       `json:"agent"`         //used to provision service in orchestrator
	BinaryLogFiles              map[string]uint              `json:"binaryLogFiles"`
}

type serverList []*ServerMonitor

const (
	stateFailed       string = "Failed"
	stateMaster       string = "Master"
	stateSlave        string = "Slave"
	stateSlaveErr     string = "SlaveErr"
	stateSlaveLate    string = "SlaveLate"
	stateMaintenance  string = "Maintenance"
	stateUnconn       string = "StandAlone"
	stateErrorAuth    string = "ErrorAuth"
	stateSuspect      string = "Suspect"
	stateShard        string = "Shard"
	stateProv         string = "Provision"
	stateMasterAlone  string = "MasterAlone"
	stateRelay        string = "Relay"
	stateRelayErr     string = "RelayErr"
	stateRelayLate    string = "RelayLate"
	stateWsrep        string = "Wsrep"
	stateWsrepDonor   string = "WsrepDonor"
	stateWsrepLate    string = "WsrepUnsync"
	stateProxyRunning string = "ProxyRunning"
	stateProxyDesync  string = "ProxyDesync"
)

const (
	ConstTLSNoConfig      string = ""
	ConstTLSOldConfig     string = "&tls=tlsconfigold"
	ConstTLSCurrentConfig string = "&tls=tlsconfig"
)

/* Initializes a server object compute if spider node*/
func (cluster *Cluster) newServerMonitor(url string, user string, pass string, compute bool, domain string) (*ServerMonitor, error) {
	var err error
	server := new(ServerMonitor)
	server.QPS = 0
	server.IsCompute = compute
	server.Domain = domain
	server.TLSConfigUsed = ConstTLSCurrentConfig
	server.CrcTable = crc64.MakeTable(crc64.ECMA)
	server.ClusterGroup = cluster
	server.DBVersion = dbhelper.NewMySQLVersion("Unknowed-0.0.0", "")
	server.Name, server.Port, server.PostgressDB = misc.SplitHostPortDB(url)
	server.ClusterGroup = cluster
	server.ServiceName = cluster.Name + "/svc/" + server.Name
	if cluster.Conf.ProvNetCNI {
		/*	if server.IsCompute && cluster.Conf.ClusterHead != "" {
				url = server.Name + "." + cluster.Conf.ClusterHead + ".svc." + server.ClusterGroup.Conf.ProvOrchestratorCluster + ":3306"
			} else {
				url = server.Name + "." + cluster.Name + ".svc." + server.ClusterGroup.Conf.ProvOrchestratorCluster + ":3306"
			}*/
		url = server.Name + server.Domain + ":3306"
	}
	server.Id = "db" + strconv.FormatUint(crc64.Checksum([]byte(cluster.Name+server.Name+server.Port), crcTable), 10)
	var sid uint64
	sid, err = strconv.ParseUint(strconv.FormatUint(crc64.Checksum([]byte(server.Name+server.Port), server.CrcTable), 10), 10, 64)
	server.ServerID = sid
	if cluster.Conf.TunnelHost != "" {
		go server.Tunnel()
	}

	server.SetCredential(url, user, pass)
	server.ReplicationSourceName = cluster.Conf.MasterConn

	server.HaveSemiSync = true
	server.HaveInnodbTrxCommit = true
	server.HaveChecksum = true
	server.HaveBinlogSync = true
	server.HaveBinlogRow = true
	server.HaveBinlogAnnotate = true
	server.HaveBinlogCompress = true
	server.HaveBinlogSlowqueries = true
	server.MxsHaveGtid = false
	// consider all nodes are maxscale to avoid sending command until discoverd
	server.IsRelay = false
	server.IsMaxscale = true
	server.IsDelayed = server.IsInDelayedHost()
	server.State = stateSuspect
	server.PrevState = stateSuspect
	server.Datadir = server.ClusterGroup.Conf.WorkingDir + "/" + server.ClusterGroup.Name + "/" + server.Host + "_" + server.Port
	if _, err := os.Stat(server.Datadir); os.IsNotExist(err) {
		os.MkdirAll(server.Datadir, os.ModePerm)
		os.MkdirAll(server.Datadir+"/log", os.ModePerm)
		os.MkdirAll(server.Datadir+"/var", os.ModePerm)
		os.MkdirAll(server.Datadir+"/init", os.ModePerm)
		os.MkdirAll(server.Datadir+"/bck", os.ModePerm)
	}

	errLogFile := server.Datadir + "/log/log_error.log"
	slowLogFile := server.Datadir + "/log/log_slow_query.log"
	if _, err := os.Stat(errLogFile); os.IsNotExist(err) {
		nofile, _ := os.OpenFile(errLogFile, os.O_WRONLY|os.O_CREATE, 0600)
		nofile.Close()
	}
	if _, err := os.Stat(slowLogFile); os.IsNotExist(err) {
		nofile, _ := os.OpenFile(slowLogFile, os.O_WRONLY|os.O_CREATE, 0600)
		nofile.Close()
	}
	server.ErrorLogTailer, _ = tail.TailFile(errLogFile, tail.Config{Follow: true, ReOpen: true})
	server.SlowLogTailer, _ = tail.TailFile(slowLogFile, tail.Config{Follow: true, ReOpen: true})
	server.ErrorLog = s18log.NewHttpLog(server.ClusterGroup.Conf.MonitorErrorLogLength)
	server.SlowLog = s18log.NewSlowLog(server.ClusterGroup.Conf.MonitorLongQueryLogLength)
	go server.ErrorLogWatcher()
	go server.SlowLogWatcher()
	server.SetIgnored(cluster.IsInIgnoredHosts(server))
	server.SetPreferedBackup(cluster.IsInPreferedBackupHosts(server))
	server.SetPrefered(cluster.IsInPreferedHosts(server))
	/*if server.ClusterGroup.Conf.MasterSlavePgStream || server.ClusterGroup.Conf.MasterSlavePgLogical {
		server.Conn, err = sqlx.Open("postgres", server.DSN)
	} else {
		server.Conn, err = sqlx.Open("mysql", server.DSN)
	}*/
	return server, err
}

func (server *ServerMonitor) Ping(wg *sync.WaitGroup) {

	defer wg.Done()

	if server.ClusterGroup.vmaster != nil {
		if server.ClusterGroup.vmaster.ServerID == server.ServerID {
			server.IsVirtualMaster = true
		} else {
			server.IsVirtualMaster = false
		}
	}
	var conn *sqlx.DB
	var err error
	switch server.ClusterGroup.Conf.CheckType {
	case "tcp":
		conn, err = server.GetNewDBConn()
	case "agent":
		var resp *http.Response
		resp, err = http.Get("http://" + server.Host + ":10001/check/")
		if resp.StatusCode != 200 {
			// if 404, consider server down or agent killed. Don't initiate anything
			err = fmt.Errorf("HTTP Response Code Error: %d", resp.StatusCode)
		}
	}
	// manage IP based DNS may failed if backend server as changed IP  try to resolv it and recreate new DSN
	//server.SetCredential(server.URL, server.User, server.Pass)
	// Handle failure cases here
	if err != nil {
		// Copy the last known server states or they will be cleared at next monitoring loop
		if server.State != stateFailed {
			server.ClusterGroup.sme.CopyOldStateFromUnknowServer(server.URL)
		}
		// server.ClusterGroup.LogPrintf(LvlDbg, "Failure detection handling for server %s %s", server.URL, err)
		// server.ClusterGroup.LogPrintf(LvlErr, "Failure detection handling for server %s %s", server.URL, err)

		if driverErr, ok := err.(*mysql.MySQLError); ok {
			//	server.ClusterGroup.LogPrintf(LvlDbg, "Driver Error %s %d ", server.URL, driverErr.Number)

			// access denied
			if driverErr.Number == 1045 {
				server.State = stateErrorAuth
				server.ClusterGroup.SetState("ERR00004", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00004"], server.URL, err.Error()), ErrFrom: "SRV"})
				return
			} else {
				server.ClusterGroup.LogPrintf(LvlErr, "Driver Error %s %d ", server.URL, driverErr.Number)
			}
		}
		if err != sql.ErrNoRows {
			server.FailCount++
			if server.ClusterGroup.master == nil {
				server.ClusterGroup.LogPrintf(LvlDbg, "Master not defined")
			}
			if server.ClusterGroup.master != nil && server.URL == server.ClusterGroup.master.URL {
				server.FailSuspectHeartbeat = server.ClusterGroup.sme.GetHeartbeats()
				if server.ClusterGroup.master.FailCount <= server.ClusterGroup.Conf.MaxFail {
					server.ClusterGroup.LogPrintf("INFO", "Master Failure detected! Retry %d/%d", server.ClusterGroup.master.FailCount, server.ClusterGroup.Conf.MaxFail)
				}
				if server.FailCount >= server.ClusterGroup.Conf.MaxFail {
					if server.FailCount == server.ClusterGroup.Conf.MaxFail {
						server.ClusterGroup.LogPrintf("INFO", "Declaring db master as failed %s", server.URL)
					}
					server.ClusterGroup.master.State = stateFailed
					server.DelWaitStopCookie()
				} else {
					server.ClusterGroup.master.State = stateSuspect

				}
			} else {
				// not the master
				server.ClusterGroup.LogPrintf(LvlDbg, "Failure detection of no master FailCount %d MaxFail %d", server.FailCount, server.ClusterGroup.Conf.MaxFail)
				if server.FailCount >= server.ClusterGroup.Conf.MaxFail {
					if server.FailCount == server.ClusterGroup.Conf.MaxFail {
						server.ClusterGroup.LogPrintf("INFO", "Declaring slave db %s as failed", server.URL)
						server.State = stateFailed
						server.DelWaitStopCookie()
						// remove from slave list
						server.delete(&server.ClusterGroup.slaves)
						if server.Replications != nil {
							server.LastSeenReplications = server.Replications
						}
						server.Replications = nil
					}
				} else {
					server.State = stateSuspect
				}
			}
		}
		// Send alert if state has changed
		if server.PrevState != server.State {
			//if cluster.Conf.Verbose {
			server.ClusterGroup.LogPrintf(LvlDbg, "Server %s state changed from %s to %s", server.URL, server.PrevState, server.State)
			if server.State != stateSuspect {
				server.ClusterGroup.LogPrintf("ALERT", "Server %s state changed from %s to %s", server.URL, server.PrevState, server.State)
				server.ClusterGroup.backendStateChangeProxies()
				server.SendAlert()
				server.ProcessFailedSlave()
			}
		}
		if server.PrevState != server.State {
			server.PrevState = server.State
		}
		return
	}

	// From here we have a new connection
	// We will affect it or closing it

	if server.ClusterGroup.sme.IsInFailover() {
		conn.Close()
		server.ClusterGroup.LogPrintf(LvlDbg, "Inside failover, skiping refresh")
		return
	}
	// reaffect a global DB pool object if we never get it , ex dynamic seeding
	if server.Conn == nil {
		server.Conn = conn
		server.ClusterGroup.LogPrintf(LvlInfo, "Assigning a global connection on server %s", server.URL)
		return
	}
	err = server.Refresh()
	if err != nil {
		// reaffect a global DB pool object if we never get it , ex dynamic seeding
		server.Conn = conn
		server.ClusterGroup.LogPrintf(LvlInfo, "Server refresh failed but ping connect %s", err)
		return
	}
	defer conn.Close()

	// For orchestrator to trigger a start via tracking state URL
	if server.PrevState == stateFailed {
		server.DelWaitStartCookie()
		server.DelRestartCookie()
	}
	// Reset FailCount
	if (server.State != stateFailed && server.State != stateErrorAuth && server.State != stateSuspect) && (server.FailCount > 0) /*&& (((server.ClusterGroup.sme.GetHeartbeats() - server.FailSuspectHeartbeat) * server.ClusterGroup.Conf.MonitoringTicker) > server.ClusterGroup.Conf.FailResetTime)*/ {
		server.FailCount = 0
		server.FailSuspectHeartbeat = 0
	}

	var ss dbhelper.SlaveStatus
	ss, _, errss := dbhelper.GetSlaveStatus(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
	// We have no replicatieon can this be the old master
	//  1617 is no multi source channel found
	noChannel := false
	if errss != nil {
		if strings.Contains(errss.Error(), "1617") {
			// This is a special case when using muti source there is a error instead of empty resultset when no replication is defined on channel
			//	server.ClusterGroup.LogPrintf(LvlInfo, " server: %s replication no channel err 1617 %s ", server.URL, errss)
			noChannel = true
		}
	}
	if errss == sql.ErrNoRows || noChannel {
		// If we reached this stage with a previously failed server, reintroduce
		// it as unconnected server.
		if server.PrevState == stateFailed || server.PrevState == stateErrorAuth {
			server.ClusterGroup.LogPrintf(LvlDbg, "State comparison reinitialized failed server %s as unconnected", server.URL)
			if server.ClusterGroup.Conf.ReadOnly && server.HaveWsrep == false && server.ClusterGroup.IsDiscovered() {
				if server.ClusterGroup.master != nil {
					if server.ClusterGroup.Status == ConstMonitorActif && server.ClusterGroup.master.Id != server.Id && !server.ClusterGroup.IsInIgnoredReadonly(server) {
						server.ClusterGroup.LogPrintf(LvlInfo, "Setting Read Only on unconnected server %s as active monitor and other master is discovered", server.URL)
						server.SetReadOnly()
					} else if server.ClusterGroup.Status == ConstMonitorStandby && server.ClusterGroup.Conf.Arbitration && !server.ClusterGroup.IsInIgnoredReadonly(server) {
						server.ClusterGroup.LogPrintf(LvlInfo, "Setting Read Only on unconnected server %s as a standby monitor ", server.URL)
						server.SetReadOnly()
					}
				}
			}
			//if server.ClusterGroup.GetTopology() != topoMultiMasterWsrep {
			server.State = stateUnconn
			//}
			server.FailCount = 0
			server.ClusterGroup.backendStateChangeProxies()
			server.SendAlert()
			if server.ClusterGroup.Conf.Autorejoin && server.ClusterGroup.IsActive() {
				server.RejoinMaster()
			} else {
				server.ClusterGroup.LogPrintf("INFO", "Auto Rejoin is disabled")
			}

		} else if server.State != stateMaster && server.PrevState != stateUnconn {
			// Master will never get discovery in topology if it does not get unconnected first it default to suspect
			if server.ClusterGroup.GetTopology() != topoMultiMasterWsrep {
				server.State = stateUnconn
				server.ClusterGroup.LogPrintf(LvlDbg, "State unconnected set by non-master rule on server %s", server.URL)
			}
			if server.ClusterGroup.Conf.ReadOnly && server.HaveWsrep == false && server.ClusterGroup.IsDiscovered() && !server.ClusterGroup.IsInIgnoredReadonly(server) {
				server.ClusterGroup.LogPrintf(LvlInfo, "Setting Read Only on unconnected server: %s no master state and replication found", server.URL)
				server.SetReadOnly()
			}

			if server.State != stateSuspect {
				server.ClusterGroup.backendStateChangeProxies()
				server.SendAlert()
			}
		}

	} else if server.ClusterGroup.IsActive() && errss == nil && (server.PrevState == stateFailed) {

		server.rejoinSlave(ss)
	}

	if server.PrevState != server.State {
		server.PrevState = server.State
		if server.PrevState != stateSuspect {
			server.ClusterGroup.backendStateChangeProxies()
			server.SendAlert()
		}
	}
}

func (server *ServerMonitor) ProcessFailedSlave() {

	if server.State == stateSlaveErr {
		if server.ClusterGroup.Conf.ReplicationErrorScript != "" {
			server.ClusterGroup.LogPrintf("INFO", "Calling replication error script")
			var out []byte
			out, err := exec.Command(server.ClusterGroup.Conf.ReplicationErrorScript, server.URL, server.PrevState, server.State).CombinedOutput()
			if err != nil {
				server.ClusterGroup.LogPrintf("ERROR", "%s", err)
			}
			server.ClusterGroup.LogPrintf("INFO", "Replication error script complete:", string(out))
		}
		if server.HasReplicationSQLThreadRunning() && server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch != "" {
			ss, err := server.GetSlaveStatus(server.ReplicationSourceName)
			if err != nil {
				return
			}
			matched, err := regexp.Match(server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch, []byte(ss.LastSQLError.String))
			if err != nil {
				server.ClusterGroup.LogPrintf("ERROR", "Rexep failed replication-restart-on-sqlerror-match %s %s", server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch, err)
			} else if matched {
				server.ClusterGroup.LogPrintf("INFO", "Rexep restart slave  %s  matching: %s", server.ClusterGroup.Conf.ReplicationRestartOnSQLErrorMatch, ss.LastSQLError.String)
				server.SkipReplicationEvent()
				server.StartSlave()
				server.ClusterGroup.LogPrintf("INFO", "Skip event and restart slave on %s", server.URL)
			}
		}
	}
}

// Refresh a server object
func (server *ServerMonitor) Refresh() error {
	var err error
	if server.Conn == nil {
		return errors.New("Connection is nil, server unreachable")
	}
	if server.Conn.Unsafe() == nil {
		//	server.State = stateFailed
		return errors.New("Connection is unsafe, server unreachable")
	}

	err = server.Conn.Ping()
	if err != nil {
		return err
	}
	server.CheckVersion()

	if server.ClusterGroup.Conf.MxsBinlogOn {
		mxsversion, _ := dbhelper.GetMaxscaleVersion(server.Conn)
		if mxsversion != "" {
			server.ClusterGroup.LogPrintf(LvlInfo, "Found Maxscale")
			server.IsMaxscale = true
			server.IsRelay = true
			server.MxsVersion = dbhelper.MariaDBVersion(mxsversion)
			server.State = stateRelay
		} else {
			server.IsMaxscale = false
		}
	} else {
		server.IsMaxscale = false
	}

	if !(server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale) {
		// maxscale don't support show variables
		server.PrevMonitorTime = server.MonitorTime
		server.MonitorTime = time.Now().Unix()
		logs := ""
		server.DBVersion, logs, err = dbhelper.GetDBVersion(server.Conn)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlErr, "Could not get database version %s %s", server.URL, err)

		server.Variables, logs, err = dbhelper.GetVariables(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlErr, "Could not get database variables %s %s", server.URL, err)
		if err != nil {
			return nil
		}
		if !server.DBVersion.IsPPostgreSQL() {

			server.HaveEventScheduler = server.HasEventScheduler()
			server.Strict = server.Variables["GTID_STRICT_MODE"]
			server.ReadOnly = server.Variables["READ_ONLY"]
			server.LongQueryTime = server.Variables["LONG_QUERY_TIME"]
			server.LogOutput = server.Variables["LOG_OUTPUT"]
			server.SlowQueryLog = server.Variables["SLOW_QUERY_LOG"]
			server.HaveReadOnly = server.HasReadOnly()
			server.HaveBinlog = server.HasBinlog()
			server.HaveBinlogRow = server.HasBinlogRow()
			server.HaveBinlogAnnotate = server.HasBinlogRowAnnotate()
			server.HaveBinlogSync = server.HasBinlogDurable()
			server.HaveBinlogCompress = server.HasBinlogCompress()
			server.HaveBinlogSlaveUpdates = server.HasBinlogSlaveUpdates()
			server.HaveBinlogSlowqueries = server.HasBinlogSlowSlaveQueries()
			server.HaveGtidStrictMode = server.HasGtidStrictMode()
			server.HaveInnodbTrxCommit = server.HasInnoDBRedoLogDurable()
			server.HaveChecksum = server.HasInnoDBChecksum()
			server.HaveWsrep = server.HasWsrep()
			server.HaveSlowQueryLog = server.HasLogSlowQuery()
			server.HavePFS = server.HasLogPFS()
			if server.HavePFS {
				server.HavePFSSlowQueryLog = server.HasLogPFSSlowQuery()
			}
			server.HaveMySQLGTID = server.HasMySQLGTID()
			server.RelayLogSize, _ = strconv.ParseUint(server.Variables["RELAY_LOG_SPACE_LIMIT"], 10, 64)

			if server.DBVersion.IsMariaDB() {
				server.GTIDBinlogPos = gtid.NewList(server.Variables["GTID_BINLOG_POS"])
				server.CurrentGtid = gtid.NewList(server.Variables["GTID_CURRENT_POS"])
				server.SlaveGtid = gtid.NewList(server.Variables["GTID_SLAVE_POS"])

			} else {
				server.GTIDBinlogPos = gtid.NewMySQLList(server.Variables["GTID_EXECUTED"])
				server.GTIDExecuted = server.Variables["GTID_EXECUTED"]
				server.CurrentGtid = gtid.NewMySQLList(server.Variables["GTID_EXECUTED"])
				server.SlaveGtid = gtid.NewList(server.Variables["GTID_SLAVE_POS"])
			}

			var sid uint64
			sid, err = strconv.ParseUint(server.Variables["SERVER_ID"], 10, 64)
			if err != nil {
				server.ClusterGroup.LogPrintf(LvlErr, "Could not parse server_id, reason: %s", err)
			}
			server.ServerID = uint64(sid)

			server.EventStatus, logs, err = dbhelper.GetEventStatus(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get events status %s %s", server.URL, err)
			if err != nil {
				server.ClusterGroup.SetState("ERR00073", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00073"], server.URL), ErrFrom: "MON"})
			}
			if server.ClusterGroup.sme.GetHeartbeats()%30 == 0 {
				server.CheckPrivileges()
			} else {
				server.ClusterGroup.sme.PreserveState("ERR00007")
				server.ClusterGroup.sme.PreserveState("ERR00006")
				server.ClusterGroup.sme.PreserveState("ERR00008")
				server.ClusterGroup.sme.PreserveState("ERR00015")
				server.ClusterGroup.sme.PreserveState("ERR00078")
				server.ClusterGroup.sme.PreserveState("ERR00009")
			}
			if server.ClusterGroup.Conf.FailEventScheduler && server.IsMaster() && !server.HasEventScheduler() {
				server.ClusterGroup.LogPrintf(LvlInfo, "Enable Event Scheduler on master")
				logs, err := server.SetEventScheduler(true)
				server.ClusterGroup.LogSQL(logs, err, server.URL, "MasterFailover", LvlErr, "Could not enable event scheduler on the  master")
			}

		} // end not postgress

		// get Users
		server.Users, logs, err = dbhelper.GetUsers(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get database users %s %s", server.URL, err)
		if server.ClusterGroup.Conf.MonitorScheduler {
			server.JobsCheckRunning()
		}

		if server.ClusterGroup.Conf.MonitorProcessList {
			server.FullProcessList, logs, err = dbhelper.GetProcesslist(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get process %s %s", server.URL, err)
			if err != nil {
				server.ClusterGroup.SetState("ERR00075", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00075"], err), ServerUrl: server.URL, ErrFrom: "MON"})
			}
		}
	}
	if server.InCaptureMode {
		server.ClusterGroup.SetState("WARN0085", state.State{ErrType: LvlInfo, ErrDesc: fmt.Sprintf(clusterError["WARN0085"], server.URL), ServerUrl: server.URL, ErrFrom: "MON"})
	}
	// SHOW MASTER STATUS
	logs := ""
	server.MasterStatus, logs, err = dbhelper.GetMasterStatus(server.Conn, server.DBVersion)
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get master status %s %s", server.URL, err)
	if err != nil {
		// binary log might be closed for that server
	} else {
		server.BinaryLogFile = server.MasterStatus.File
		if server.BinaryLogFilePrevious != "" && server.BinaryLogFilePrevious != server.BinaryLogFile {
			server.BinaryLogFiles, logs, err = dbhelper.GetBinaryLogs(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get binary log files %s %s", server.URL, err)
			if server.BinaryLogFilePrevious != "" {
				server.JobBackupBinlog(server.BinaryLogFilePrevious)
				go server.JobBackupBinlogPurge(server.BinaryLogFilePrevious)
			}
		}
		server.BinaryLogFilePrevious = server.BinaryLogFile
		server.BinaryLogPos = strconv.FormatUint(uint64(server.MasterStatus.Position), 10)
	}

	if !server.DBVersion.IsPPostgreSQL() {
		server.BinlogDumpThreads, logs, err = dbhelper.GetBinlogDumpThreads(server.Conn, server.DBVersion)
		if err != nil {
			if strings.Contains(err.Error(), "Errcode: 28 ") || strings.Contains(err.Error(), "errno: 28 ") {
				// No space left on device
				server.IsFull = true
				server.ClusterGroup.SetState("WARN0100", state.State{ErrType: LvlWarn, ErrDesc: fmt.Sprintf(clusterError["WARN0100"], server.URL, err), ServerUrl: server.URL, ErrFrom: "CONF"})
				return nil
			}
		}
		server.IsFull = false
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get binoDumpthreads status %s %s", server.URL, err)
		if err != nil {
			server.ClusterGroup.SetState("ERR00014", state.State{ErrType: LvlErr, ErrDesc: fmt.Sprintf(clusterError["ERR00014"], server.URL, err), ServerUrl: server.URL, ErrFrom: "CONF"})
		}

		if server.ClusterGroup.Conf.MonitorInnoDBStatus {
			// SHOW ENGINE INNODB STATUS
			server.EngineInnoDB, logs, err = dbhelper.GetEngineInnoDBVariables(server.Conn)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get engine innodb status %s %s", server.URL, err)
		}
		if server.ClusterGroup.Conf.MonitorPFS {
			// GET PFS query digest
			server.PFSQueries, logs, err = dbhelper.GetQueries(server.Conn)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get queries %s %s", server.URL, err)
		}
		if server.HaveDiskMonitor {
			server.Disks, logs, err = dbhelper.GetDisks(server.Conn, server.DBVersion)
		}
		if server.ClusterGroup.Conf.MonitorScheduler {
			server.CheckDisks()
		}
		if server.HasLogsInSystemTables() {
			go server.GetSlowLogTable()
		}

	} // End not PG

	// Set channel source name is dangerous with multi cluster

	// SHOW SLAVE STATUS

	if !(server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale) && server.DBVersion.IsMariaDB() || server.DBVersion.IsPPostgreSQL() {
		server.Replications, logs, err = dbhelper.GetAllSlavesStatus(server.Conn, server.DBVersion)
		if len(server.Replications) > 0 && err == nil && server.DBVersion.IsPPostgreSQL() && server.ReplicationSourceName == "" {
			//setting first subscription if we don't have one
			server.ReplicationSourceName = server.Replications[0].ConnectionName.String
		}
	} else {
		server.Replications, logs, err = dbhelper.GetChannelSlaveStatus(server.Conn, server.DBVersion)
	}
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get slaves status %s %s", server.URL, err)

	// select a replication status get an err if repliciations array is empty
	server.SlaveStatus, err = server.GetSlaveStatus(server.ReplicationSourceName)
	if err != nil {
		// Do not reset  server.MasterServerID = 0 as we may need it for recovery
		server.IsSlave = false
	} else {

		server.IsSlave = true
		if server.DBVersion.IsPPostgreSQL() {
			//PostgresQL as no server_id concept mimic via internal server id for topology detection
			var sid uint64
			sid, err = strconv.ParseUint(strconv.FormatUint(crc64.Checksum([]byte(server.SlaveStatus.MasterHost.String+server.SlaveStatus.MasterPort.String), server.CrcTable), 10), 10, 64)
			if err != nil {
				server.ClusterGroup.LogPrintf(LvlWarn, "PG Could not assign server_id s", err)
			}
			server.SlaveStatus.MasterServerID = sid
			for i := range server.Replications {
				server.Replications[i].MasterServerID = sid
			}

			server.SlaveGtid = gtid.NewList(server.SlaveStatus.GtidSlavePos.String)

		} else {
			if server.SlaveStatus.UsingGtid.String == "Slave_Pos" || server.SlaveStatus.UsingGtid.String == "Current_Pos" {
				server.HaveMariaDBGTID = true
			} else {
				server.HaveMariaDBGTID = false
			}
			if server.DBVersion.IsMySQLOrPerconaGreater57() && server.HasGTIDReplication() {
				server.SlaveGtid = gtid.NewList(server.SlaveStatus.ExecutedGtidSet.String)
			}
		}
	}
	server.ReplicationHealth = server.CheckReplication()
	// if MaxScale exit at fetch variables and status part as not supported

	if server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale {
		return nil
	}
	server.PrevStatus = server.Status

	server.Status, logs, _ = dbhelper.GetStatus(server.Conn, server.DBVersion)
	//server.ClusterGroup.LogPrintf("ERROR: %s %s %s", su["RPL_SEMI_SYNC_MASTER_STATUS"], su["RPL_SEMI_SYNC_SLAVE_STATUS"], server.URL)
	if server.Status["RPL_SEMI_SYNC_MASTER_STATUS"] == "" || server.Status["RPL_SEMI_SYNC_SLAVE_STATUS"] == "" {
		server.HaveSemiSync = false
	} else {
		server.HaveSemiSync = true
	}
	if server.Status["RPL_SEMI_SYNC_MASTER_STATUS"] == "ON" {
		server.SemiSyncMasterStatus = true
	} else {
		server.SemiSyncMasterStatus = false
	}
	if server.Status["RPL_SEMI_SYNC_SLAVE_STATUS"] == "ON" {
		server.SemiSyncSlaveStatus = true
	} else {
		server.SemiSyncSlaveStatus = false
	}

	if server.Status["WSREP_LOCAL_STATE"] == "4" {
		server.IsWsrepSync = true
	} else {
		server.IsWsrepSync = false
	}
	if server.Status["WSREP_LOCAL_STATE"] == "2" {
		server.IsWsrepDonor = true
	} else {
		server.IsWsrepDonor = false
	}
	if server.Status["WSREP_CLUSTER_STATUS"] == "PRIMARY" {
		server.IsWsrepPrimary = true
	} else {
		server.IsWsrepPrimary = false
	}
	if len(server.PrevStatus) > 0 {
		qps, _ := strconv.ParseInt(server.Status["QUERIES"], 10, 64)
		prevqps, _ := strconv.ParseInt(server.PrevStatus["QUERIES"], 10, 64)
		if server.MonitorTime-server.PrevMonitorTime > 0 {
			server.QPS = (qps - prevqps) / (server.MonitorTime - server.PrevMonitorTime)
		}
	}

	if server.HasHighNumberSlowQueries() {
		server.ClusterGroup.SetState("WARN0088", state.State{ErrType: LvlInfo, ErrDesc: fmt.Sprintf(clusterError["WARN0088"], server.URL), ServerUrl: server.URL, ErrFrom: "MON"})
	}
	// monitor plugins
	if !server.DBVersion.IsPPostgreSQL() {
		if server.ClusterGroup.sme.GetHeartbeats()%60 == 0 {
			if server.ClusterGroup.Conf.MonitorPlugins {
				server.Plugins, logs, err = dbhelper.GetPlugins(server.Conn, server.DBVersion)
				server.HaveMetaDataLocksLog = server.HasInstallPlugin("METADATA_LOCK_INFO")
				server.HaveQueryResponseTimeLog = server.HasInstallPlugin("QUERY_RESPONSE_TIME")
				server.HaveDiskMonitor = server.HasInstallPlugin("DISK")
				server.HaveSQLErrorLog = server.HasInstallPlugin("SQL_ERROR_LOG")
			}
			server.BinlogDumpThreads, logs, err = dbhelper.GetBinlogDumpThreads(server.Conn, server.DBVersion)
			if err != nil {
				if strings.Contains(err.Error(), "Errcode: 28 ") || strings.Contains(err.Error(), "errno: 28 ") {
					// No space left on device
					server.IsFull = true
					server.ClusterGroup.SetState("WARN0100", state.State{ErrType: LvlWarn, ErrDesc: fmt.Sprintf(clusterError["WARN0100"], server.URL, err), ServerUrl: server.URL, ErrFrom: "CONF"})
					return nil
				} else {
					server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get plugins  %s %s", server.URL, err)
				}
			}
			server.IsFull = false
		}
		if server.HaveMetaDataLocksLog {
			server.MetaDataLocks, logs, err = dbhelper.GetMetaDataLock(server.Conn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Monitor", LvlDbg, "Could not get Metat data locks  %s %s", server.URL, err)
		}
	}
	server.CheckMaxConnections()

	// Initialize graphite monitoring
	if server.ClusterGroup.Conf.GraphiteMetrics {
		go server.SendDatabaseStats()
	}
	return nil
}

/* Handles write freeze and existing transactions on a server */
func (server *ServerMonitor) freeze() bool {
	logs, err := dbhelper.SetReadOnly(server.Conn, true)
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlInfo, "Could not set %s as read-only: %s", server.URL, err)
	if err != nil {
		return false
	}
	for i := server.ClusterGroup.Conf.SwitchWaitKill; i > 0; i -= 500 {
		threads, logs, err := dbhelper.CheckLongRunningWrites(server.Conn, 0)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlErr, "Could not check long running Writes %s as read-only: %s", server.URL, err)
		if threads == 0 {
			break
		}
		server.ClusterGroup.LogPrintf(LvlInfo, "Waiting for %d write threads to complete on %s", threads, server.URL)
		time.Sleep(500 * time.Millisecond)
	}
	server.maxConn, logs, err = dbhelper.GetVariableByName(server.Conn, "MAX_CONNECTIONS", server.DBVersion)
	server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlErr, "Could not get max_connections value on demoted leader")
	if err != nil {

	} else {
		if server.ClusterGroup.Conf.SwitchDecreaseMaxConn {
			logs, err := dbhelper.SetMaxConnections(server.Conn, strconv.FormatInt(server.ClusterGroup.Conf.SwitchDecreaseMaxConnValue, 10), server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "Freeze", LvlErr, "Could not set max_connections to 1 on demoted leader %s %s", server.URL, err)
		}
	}
	server.ClusterGroup.LogPrintf("INFO", "Terminating all threads on %s", server.URL)
	dbhelper.KillThreads(server.Conn, server.DBVersion)
	return true
}

func (server *ServerMonitor) ReadAllRelayLogs() error {

	server.ClusterGroup.LogPrintf(LvlInfo, "Reading all relay logs on %s", server.URL)
	if server.DBVersion.IsMariaDB() && server.HaveMariaDBGTID {
		ss, logs, err := dbhelper.GetMSlaveStatus(server.Conn, "", server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get slave status %s %s", server.URL, err)
		if err != nil {
			return err
		}
		server.Refresh()
		myGtid_IO_Pos := gtid.NewList(ss.GtidIOPos.String)
		myGtid_Slave_Pos := server.SlaveGtid
		//myGtid_Slave_Pos := gtid.NewList(ss.GtidSlavePos.String)
		//https://jira.mariadb.org/browse/MDEV-14182

		for myGtid_Slave_Pos.Equal(myGtid_IO_Pos) == false && ss.UsingGtid.String != "" && ss.GtidSlavePos.String != "" && server.State != stateFailed {
			server.Refresh()
			ss, logs, err = dbhelper.GetMSlaveStatus(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get slave status %s %s", server.URL, err)

			if err != nil {
				return err
			}
			time.Sleep(500 * time.Millisecond)
			myGtid_IO_Pos = gtid.NewList(ss.GtidIOPos.String)
			myGtid_Slave_Pos = server.SlaveGtid

			server.ClusterGroup.LogPrintf(LvlInfo, "Waiting sync IO_Pos:%s, Slave_Pos:%s", myGtid_IO_Pos.Sprint(), myGtid_Slave_Pos.Sprint())
		}
	} else {
		ss, logs, err := dbhelper.GetSlaveStatus(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get slave status %s %s", server.URL, err)
		if err != nil {
			return err
		}
		for true {
			server.ClusterGroup.LogPrintf(LvlInfo, "Waiting sync IO_Pos:%s/%s, Slave_Pos:%s %s", ss.MasterLogFile, ss.ReadMasterLogPos.String, ss.RelayMasterLogFile, ss.ExecMasterLogPos.String)
			if ss.MasterLogFile == ss.RelayMasterLogFile && ss.ReadMasterLogPos == ss.ExecMasterLogPos {
				break
			}
			ss, logs, err = dbhelper.GetSlaveStatus(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
			server.ClusterGroup.LogSQL(logs, err, server.URL, "ReadAllRelayLogs", LvlErr, "Could not get slave status %s %s", server.URL, err)
			if err != nil {
				return err
			}
			if strings.Contains(ss.SlaveSQLRunningState.String, "Slave has read all relay log") {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}
	}
	return nil
}

func (server *ServerMonitor) LogReplPostion() {
	server.Refresh()
	server.ClusterGroup.LogPrintf(LvlInfo, "Server:%s Current GTID:%s Slave GTID:%s Binlog Pos:%s", server.URL, server.CurrentGtid.Sprint(), server.SlaveGtid.Sprint(), server.GTIDBinlogPos.Sprint())
	return
}

func (server *ServerMonitor) Close() {
	server.Conn.Close()
	return
}

func (server *ServerMonitor) writeState() error {
	server.LogReplPostion()
	f, err := os.Create("/tmp/repmgr.state")
	if err != nil {
		return err
	}
	_, err = f.WriteString(server.GTIDBinlogPos.Sprint())
	if err != nil {
		return err
	}
	return nil
}

func (server *ServerMonitor) delete(sl *serverList) {
	lsm := *sl
	for k, s := range lsm {
		if server.URL == s.URL {
			lsm[k] = lsm[len(lsm)-1]
			lsm[len(lsm)-1] = nil
			lsm = lsm[:len(lsm)-1]
			break
		}
	}
	*sl = lsm
}

func (server *ServerMonitor) StopSlave() (string, error) {
	if server.Conn == nil {
		return "", errors.New("No database connection pool")
	}
	return dbhelper.StopSlave(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
}

func (server *ServerMonitor) StartSlave() (string, error) {
	if server.Conn == nil {
		return "", errors.New("No databse connection")
	}
	return dbhelper.StartSlave(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)

}

func (server *ServerMonitor) ResetMaster() (string, error) {
	if server.Conn == nil {
		return "", errors.New("No database connection pool")
	}
	return dbhelper.ResetMaster(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
}

func (server *ServerMonitor) ResetPFSQueries() error {
	return server.ExecQueryNoBinLog("truncate performance_schema.events_statements_summary_by_digest")
}

func (server *ServerMonitor) StopSlaveIOThread() (string, error) {
	if server.Conn == nil {
		return "", errors.New("No database connection pool")
	}
	return dbhelper.StopSlaveIOThread(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
}

func (server *ServerMonitor) StopSlaveSQLThread() (string, error) {
	if server.Conn == nil {
		return "", errors.New("No database connection pool")
	}
	return dbhelper.StopSlaveSQLThread(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
}

func (server *ServerMonitor) ResetSlave() (string, error) {
	return dbhelper.ResetSlave(server.Conn, true, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
}

func (server *ServerMonitor) FlushLogs() (string, error) {
	if server.Conn == nil {
		return "", errors.New("No database connection pool")
	}
	return dbhelper.FlushLogs(server.Conn)
}

func (server *ServerMonitor) FlushTables() (string, error) {
	if server.Conn == nil {
		return "", errors.New("No database connection pool")
	}
	return dbhelper.FlushTables(server.Conn)
}

func (server *ServerMonitor) Uprovision() {
	server.ClusterGroup.OpenSVCUnprovisionDatabaseService(server)
}

func (server *ServerMonitor) Provision() {
	server.ClusterGroup.OpenSVCProvisionDatabaseService(server)
}

func (server *ServerMonitor) SkipReplicationEvent() {
	server.StopSlave()
	dbhelper.SkipBinlogEvent(server.Conn, server.ClusterGroup.Conf.MasterConn, server.DBVersion)
	server.StartSlave()
}

func (server *ServerMonitor) KillThread(id string) (string, error) {
	return dbhelper.KillThread(server.Conn, id, server.DBVersion)
}

func (server *ServerMonitor) KillQuery(id string) (string, error) {
	return dbhelper.KillQuery(server.Conn, id, server.DBVersion)
}

func (server *ServerMonitor) ExecQueryNoBinLog(query string) error {
	Conn, err := server.GetNewDBConn()
	if err != nil {
		server.ClusterGroup.LogPrintf(LvlErr, "Error connection in exec query no log %s %s", query, err)
		return err
	}
	defer Conn.Close()
	_, err = Conn.Exec("set sql_log_bin=0")
	if err != nil {
		server.ClusterGroup.LogPrintf(LvlErr, "Error disabling binlog %s", err)
		return err
	}
	_, err = Conn.Exec(query)
	if err != nil {
		server.ClusterGroup.LogPrintf(LvlErr, "Error query %s %s", query, err)
		return err
	}
	return err
}

func (server *ServerMonitor) ExecScriptSQL(queries []string) (error, bool) {
	hasreadonlyvar := false
	if server.State == stateFailed {
		errmsg := "Can't execute script on failed server: " + server.URL
		return errors.New(errmsg), hasreadonlyvar
	}
	for _, query := range queries {
		if strings.Trim(query, " ") == "" {
			continue
		}
		_, err := server.Conn.Exec(query)
		if err != nil {
			server.ClusterGroup.LogPrintf(LvlErr, "Apply config: %s %s", query, err)
			if driverErr, ok := err.(*mysql.MySQLError); ok {
				// access denied
				if driverErr.Number == 1238 {
					hasreadonlyvar = true
				}
			}
		}
		server.ClusterGroup.LogPrintf(LvlInfo, "Apply dynamic config: %s", query)
	}
	return nil, hasreadonlyvar
}

func (server *ServerMonitor) InstallPlugin(name string) error {
	val, ok := server.Plugins[name]

	if !ok {
		return errors.New("Plugin not loaded")
	} else {
		if val.Status == "NOT INSTALLED" {
			query := "INSTALL PLUGIN " + name + " SONAME '" + val.Library.String + "'"
			err := server.ExecQueryNoBinLog(query)
			if err != nil {
				return err
			}
			val.Status = "ACTIVE"
			server.Plugins[name] = val
		} else {
			return errors.New("Already Install Plugin")
		}
	}
	return nil
}

func (server *ServerMonitor) UnInstallPlugin(name string) error {
	val, ok := server.Plugins[name]
	if !ok {
		return errors.New("Plugin not loaded")
	} else {
		if val.Status == "ACTIVE" {
			query := "UNINSTALL PLUGIN " + name
			err := server.ExecQueryNoBinLog(query)
			if err != nil {
				return err
			}
			val.Status = "NOT INSTALLED"
			server.Plugins[name] = val
		} else {
			return errors.New("Already not installed Plugin")
		}
	}
	return nil
}

func (server *ServerMonitor) Capture() error {

	if server.InCaptureMode {
		return nil
	}

	go server.CaptureLoop(server.ClusterGroup.GetStateMachine().GetHeartbeats())
	go server.JobCapturePurge(server.ClusterGroup.Conf.WorkingDir+"/"+server.ClusterGroup.Name, server.ClusterGroup.Conf.MonitorCaptureFileKeep)
	return nil
}

func (server *ServerMonitor) CaptureLoop(start int64) {
	server.InCaptureMode = true

	type Save struct {
		ProcessList  []dbhelper.Processlist `json:"processlist"`
		InnoDBStatus string                 `json:"innodbstatus"`
		Status       map[string]string      `json:"status"`
		SlaveSatus   []dbhelper.SlaveStatus `json:"slavestatus"`
	}

	t := time.Now()
	logs := ""
	var err error
	for true {

		var clsave Save
		clsave.ProcessList,
			logs, err = dbhelper.GetProcesslist(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed Processlist for server %s: %s ", server.URL, err)

		clsave.InnoDBStatus, logs, err = dbhelper.GetEngineInnoDBSatus(server.Conn)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed InnoDB Status for server %s: %s ", server.URL, err)
		clsave.Status, logs, err = dbhelper.GetStatus(server.Conn, server.DBVersion)
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed Status for server %s: %s ", server.URL, err)

		if !(server.ClusterGroup.Conf.MxsBinlogOn && server.IsMaxscale) && server.DBVersion.IsMariaDB() {
			clsave.SlaveSatus, logs, err = dbhelper.GetAllSlavesStatus(server.Conn, server.DBVersion)
		} else {
			clsave.SlaveSatus, logs, err = dbhelper.GetChannelSlaveStatus(server.Conn, server.DBVersion)
		}
		server.ClusterGroup.LogSQL(logs, err, server.URL, "CaptureLoop", LvlErr, "Failed Slave Status for server %s: %s ", server.URL, err)

		saveJSON, _ := json.MarshalIndent(clsave, "", "\t")
		err := ioutil.WriteFile(server.ClusterGroup.Conf.WorkingDir+"/"+server.ClusterGroup.Name+"/capture_"+server.Name+"_"+t.Format("20060102150405")+".json", saveJSON, 0644)
		if err != nil {
			return
		}
		if server.ClusterGroup.GetStateMachine().GetHeartbeats() < start+5 {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	server.InCaptureMode = false
}

func (server *ServerMonitor) RotateSystemLogs() {
	server.ClusterGroup.LogPrintf(LvlInfo, "Log rotate on %s", server.URL)

	if server.HasLogsInSystemTables() && !server.IsDown() {
		if server.HasLogSlowQuery() {
			server.RotateTableToTime("mysql", "slow_log")
		}
		if server.HasLogGeneral() {
			server.RotateTableToTime("mysql", "general_log")
		}
	}
}

func (server *ServerMonitor) RotateTableToTime(database string, table string) {
	currentTime := time.Now()
	timeStampString := currentTime.Format("20060102150405")
	newtablename := table + "_" + timeStampString
	temptable := table + "_temp"
	query := "CREATE TABLE IF NOT EXISTS " + database + "." + temptable + " LIKE " + database + "." + table
	server.ExecQueryNoBinLog(query)
	query = "RENAME TABLE  " + database + "." + table + " TO " + database + "." + newtablename + " , " + database + "." + temptable + " TO " + database + "." + table
	server.ExecQueryNoBinLog(query)
	query = "select table_name from information_schema.tables where table_schema='" + database + "' and table_name like '" + table + "_%' order by table_name desc limit " + strconv.Itoa(server.ClusterGroup.Conf.SchedulerMaintenanceDatabaseLogsTableKeep) + ",100"
	cleantables := []string{}

	err := server.Conn.Select(&cleantables, query)
	if err != nil {
		return
	}
	for _, row := range cleantables {
		server.ExecQueryNoBinLog("DROP TABLE " + database + "." + row)
	}
}

func (server *ServerMonitor) WaitInnoDBPurge() error {
	query := "SET GLOBAL innodb_purge_rseg_truncate_frequency=1"
	server.ExecQueryNoBinLog(query)
	ct := 0
	for {
		if server.EngineInnoDB["history_list_lenght_inside_innodb"] == "0" {
			return nil
		}
		if ct == 1200 {
			return errors.New("Waiting to long for history_list_lenght_inside_innodb 0")
		}
	}
}

func (server *ServerMonitor) Shutdown() error {
	if server.Conn == nil {
		return errors.New("No database connection pool")
	}
	cmd := "SHUTDOWN"
	if server.DBVersion.IsMariaDB() && server.DBVersion.Major >= 10 && server.DBVersion.Minor >= 4 {
		cmd = "SHUTDOWN WAIT FOR ALL SLAVES"
	}
	_, err := server.Conn.Exec(cmd)
	if err != nil {
		server.ClusterGroup.LogPrintf("TEST", "Shutdown failed %s", err)
		return err
	}
	return nil
}

func (server *ServerMonitor) ChangeMasterTo(master *ServerMonitor, master_use_gitd string) error {
	logs := ""
	var err error
	hasMyGTID := server.HasMySQLGTID()
	//mariadb

	if server.State != stateFailed && server.ClusterGroup.Conf.ForceSlaveNoGtid == false && server.DBVersion.IsMariaDB() && server.DBVersion.Major >= 10 {
		master.Refresh()
		_, err = server.Conn.Exec("SET GLOBAL gtid_slave_pos = \"" + master.CurrentGtid.Sprint() + "\"")
		if err != nil {
			return err
		}
		logs, err = dbhelper.ChangeMaster(server.Conn, dbhelper.ChangeMasterOpt{
			Host:        master.Host,
			Port:        master.Port,
			User:        server.ClusterGroup.rplUser,
			Password:    server.ClusterGroup.rplPass,
			Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSlaveHeartbeatRetry),
			Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSlaveHeartbeatTime),
			Mode:        master_use_gitd,
			Channel:     server.ClusterGroup.Conf.MasterConn,
			IsDelayed:   server.IsDelayed,
			Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
			SSL:         server.ClusterGroup.Conf.ReplicationSSL,
			PostgressDB: server.PostgressDB,
		}, server.DBVersion)
		server.ClusterGroup.LogPrintf(LvlInfo, "Replication bootstrapped with %s as master", master.URL)
	} else if hasMyGTID && server.ClusterGroup.Conf.ForceSlaveNoGtid == false {

		logs, err = dbhelper.ChangeMaster(server.Conn, dbhelper.ChangeMasterOpt{
			Host:        master.Host,
			Port:        master.Port,
			User:        server.ClusterGroup.rplUser,
			Password:    server.ClusterGroup.rplPass,
			Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSlaveHeartbeatRetry),
			Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSlaveHeartbeatTime),
			Mode:        "MASTER_AUTO_POSITION",
			IsDelayed:   server.IsDelayed,
			Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
			SSL:         server.ClusterGroup.Conf.ReplicationSSL,
			Channel:     server.ClusterGroup.Conf.MasterConn,
			PostgressDB: server.PostgressDB,
		}, server.DBVersion)
		server.ClusterGroup.LogPrintf(LvlInfo, "Replication bootstrapped with MySQL GTID replication style and %s as master", master.URL)

	} else {
		logs, err = dbhelper.ChangeMaster(server.Conn, dbhelper.ChangeMasterOpt{
			Host:        master.Host,
			Port:        master.Port,
			User:        server.ClusterGroup.rplUser,
			Password:    server.ClusterGroup.rplPass,
			Retry:       strconv.Itoa(server.ClusterGroup.Conf.ForceSlaveHeartbeatRetry),
			Heartbeat:   strconv.Itoa(server.ClusterGroup.Conf.ForceSlaveHeartbeatTime),
			Mode:        "POSITIONAL",
			Logfile:     master.BinaryLogFile,
			Logpos:      master.BinaryLogPos,
			Channel:     server.ClusterGroup.Conf.MasterConn,
			IsDelayed:   server.IsDelayed,
			Delay:       strconv.Itoa(server.ClusterGroup.Conf.HostsDelayedTime),
			SSL:         server.ClusterGroup.Conf.ReplicationSSL,
			PostgressDB: server.PostgressDB,
		}, server.DBVersion)
		server.ClusterGroup.LogPrintf(LvlInfo, "Replication bootstrapped with old replication style and %s as master", master.URL)

	}
	if err != nil {
		server.ClusterGroup.LogSQL(logs, err, server.URL, "BootstrapReplication", LvlErr, "Replication can't be bootstrap for server %s with %s as master: %s ", server.URL, master.URL, err)
	}
	_, err = server.Conn.Exec("START SLAVE '" + server.ClusterGroup.Conf.MasterConn + "'")
	if err != nil {
		err = errors.New(fmt.Sprintln("Can't start slave: ", err))
	}
	return err
}
