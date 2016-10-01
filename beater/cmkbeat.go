package beater

import (
	"fmt"
	"time"
	"strings"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/publisher"

	"github.com/jeremyweader/cmkbeat/config"

	livestatus "github.com/vbatoufflet/go-livestatus"
)

type Cmkbeat struct {
	done   chan struct{}
	config config.Config
	client publisher.Client
}

func New(b *beat.Beat, cfg *common.Config) (beat.Beater, error) {
	config := config.DefaultConfig
	if err := cfg.Unpack(&config); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	bt := &Cmkbeat{
		done: make(chan struct{}),
		config: config,
	}
	return bt, nil
}

func (bt *Cmkbeat) Run(b *beat.Beat) error {

	bt.client = b.Publisher.Connect()
	ticker := time.NewTicker(bt.config.Period)

	for {
		select {
			case <-bt.done:
				return nil
			case <-ticker.C:
		}

		err := bt.lsQuery(bt.config.Cmkhost, b.Name)
		if err != nil {
			logp.Warn("Error executing query: %s", err)
			return err
		}
	}
}

func (bt *Cmkbeat) Stop() {
	bt.client.Close()
	close(bt.done)
}

func (bt *Cmkbeat) lsQuery(lshost string, beatname string) error {
	
	defer func() {
		if err := recover(); err != nil {
			logp.Warn("Error: %s", err)
		}
    }()
	
    start := time.Now()

    l := livestatus.NewLivestatus("tcp", lshost)
    q := l.Query("services")
    q.Columns("host_name", "description", "state", "plugin_output", "perf_data")

    resp, err := q.Exec()
    if err != nil {
		return err
    }

    var numRecords int = 0
    var errMsg string

    for _, r := range resp.Records {
        host_name, err := r.GetString("host_name")
		if err != nil {
			logp.Warn("Problem parsing response fields: %s", err)
			errMsg = err.Error()
		}
		description, err := r.GetString("description")
		if err != nil {
			logp.Warn("Problem parsing response fields: %s", err)
			errMsg = err.Error()
		}
		state, err := r.GetInt("state")
		if err != nil {
			logp.Warn("Problem parsing response fields: %s", err)
			errMsg = err.Error()
		}
		plugin_output, err := r.GetString("plugin_output")
		if err != nil {
			logp.Warn("Problem parsing response fields: %s", err)
			errMsg = err.Error()
		}
		perf_data, err := r.GetString("perf_data")
		if err != nil {
			logp.Warn("Problem parsing response fields: %s", err)
			errMsg = err.Error()
		}
		
		var perfObjMap map[string]map[string]string
		//var perfObjMap common.MapStr
		var perfDataSplit []string
		
		perfDataSplit = strings.Split(perf_data, " ")
		perfObjMap = make(map[string]map[string]string)
		for _, perfObj := range perfDataSplit {
			var perfObjSplit []string
			var dataSplit []string
			perfObjSplit = strings.Split(perfObj, "=")
			item := perfObjSplit[0]
			data := perfObjSplit[1]
			fmt.Printf("item is %s, data is %s", item, data)
			dataSplit = strings.Split(data, ";")
			fmt.Printf("length is %v", len(dataSplit))
			perfObjMap[item] = make(map[string]string)
			if len(dataSplit) >= 1 {
				perfObjMap[item]["value"] = dataSplit[0]
			}
			if len(dataSplit) >= 2 {
				perfObjMap[item]["min"] = dataSplit[1]
			}
			if len(dataSplit) >= 3 {
				perfObjMap[item]["max"] = dataSplit[2]
			}
			if len(dataSplit) >= 4 {
				perfObjMap[item]["warn"] = dataSplit[3]
			}
			if len(dataSplit) >= 5 {
				perfObjMap[item]["crit"] = dataSplit[4]
			}
		}

		event := common.MapStr {
			"@timestamp":	common.Time(time.Now()),
			"type":		beatname,
			"host":		host_name,
			"description":	description,
			"state":	state,
			"output":	plugin_output,
			"perfdata":	perf_data,
		}
		
		if errMsg != "" {
			event["error"] = errMsg
		}
		
		event["metrics"] = perfObjMap
	
		bt.client.PublishEvent(event)
		numRecords++
    }

    elapsed := time.Since(start)
    logp.Info("%v events submitted in %s.", numRecords, elapsed)
    return nil
}

