package shedule

import (
	"encoding/json"
	"fmt"
	"github.com/imroc/req"
	"github.com/meklis/http-snmpwalk-proxy/logger"
	"github.com/ztrue/tracerr"
	"strings"
	"time"
)

type ApiResponse struct {
	Code  int         `json:"code"`
	Data  interface{} `json:"data"`
	Error string      `json:"errorMessage"`
	Debug []struct {
		Time  string `json:"time"`
		Msg   string `json:"msg"`
		Level int    `json:"level"`
	} `json:"debug"`
}

type SheduleTask struct {
	ID        int                    `json:"id"`
	Generator int                    `json:"generator"`
	Method    string                 `json:"method"`
	Request   map[string]interface{} `json:"request"`
	Created   string                 `json:"created"`
}

type SheduleConfig struct {
	CountRunners int           `yaml:"count_runners"`
	CheckTime    time.Duration `yaml:"check_time"`
	ApiUrl       string        `yaml:"api_url"`
	TimeOut      time.Duration `yaml:"api_request_timeout"`
}

type Shedule struct {
	conf    SheduleConfig
	lg      *logger.Logger
	channel chan SheduleTask
}

func Init(conf SheduleConfig, lg *logger.Logger) (s *Shedule) {
	s = new(Shedule)
	s.conf = conf
	s.lg = lg
	s.channel = make(chan SheduleTask, conf.CountRunners)
	req.SetTimeout(conf.TimeOut)
	return s
}

func (s *Shedule) Run() {
	for i := 0; i < s.conf.CountRunners; i++ {
		s.lg.InfoF("[Initializer] start shedule runner #%v", i)
		go s.runner(i)
	}
	errCounter := 0
	s.lg.InfoF("[Initializer] start checker")
	for {
		time.Sleep(time.Duration(errCounter) * time.Second)
		s.lg.InfoF("Get info from %v", fmt.Sprintf("%v/shedule/get", s.conf.ApiUrl))
		resp, err := req.Get(fmt.Sprintf("%v/shedule/get", s.conf.ApiUrl))
		if err != nil {
			s.lg.Errorf("[Checker] error get job from % - err: %v", fmt.Sprintf("%v/get", s.conf.ApiUrl), err.Error())
			if errCounter < 60 {
				errCounter++
			}
			continue
		} else if resp.Response().StatusCode > 300 {
			s.lg.Errorf("[Checker] error get job from % - http err: %v - %v", fmt.Sprintf("%v/get", s.conf.ApiUrl), resp.Response().StatusCode, resp.Response().Status)
			if errCounter < 60 {
				errCounter++
			}
			continue
		}
		jresp := ApiResponse{}
		if err := resp.ToJSON(&jresp); err != nil {
			s.lg.Errorf("[Checker] error parse json response from api server: %v", err.Error())
			if errCounter < 60 {
				errCounter++
			}
			continue
		}

		errCounter = 0
		if jresp.Code == 0 {
			err, task := _parseTask(jresp.Data)
			if err != nil {
				s.lg.Errorf("[Checker] error read data field from task: %v", tracerr.Sprint(err))
			}
			s.lg.DebugF("[Checker] new task with id %v", task.ID)
			s.channel <- task
			continue
		} else if jresp.Code == 204 {
			time.Sleep(s.conf.CheckTime)
			continue
		} else if jresp.Error != "" {
			s.lg.Errorf("[Checker] %v - %v", jresp.Code, jresp.Error)
		} else {
			s.lg.Errorf("[Checker] unknown code from api: %v", jresp.Code)
		}
		time.Sleep(s.conf.CheckTime)
	}
}
func _parseTask(data interface{}) (error, SheduleTask) {
	task := SheduleTask{}
	byt, err := json.Marshal(data)
	if err != nil {
		return tracerr.Wrap(err), task
	}
	if err := json.Unmarshal(byt, &task); err != nil {
		return tracerr.Wrap(err), task
	}
	return nil, task
}

func (s *Shedule) runner(runnerNum int) {
	for {
		select {
		case task := <-s.channel:
			s.lg.NoticeF("[Runner %v] received new task - id: %v, method: %v", runnerNum, task.ID, task.Method)
			err, code, response := s.execTask(task)
			if err != nil {
				s.lg.Errorf("[Runner %v-%v] executor returned err: %v", runnerNum, task.ID, tracerr.Sprint(err))
				response = tracerr.Sprint(err)
			} else if code != 0 {
				s.lg.WarningF("[Runner %v-%v] task returned code %v with message %v", runnerNum, task.ID, code, response)
			}
			for {
				if err := s.sendTaskResponse(task.ID, code, response); err != nil {
					s.lg.Errorf("[Runner %v-%v] error update task status: %v", runnerNum, task.ID, err)
				} else {
					break
				}
			}
		default:
			time.Sleep(s.conf.CheckTime)
		}
	}
}

func wrapToValue(val interface{}) string {
	retVal := ""
	switch val.(type) {
	case float64:
		retVal = strings.Trim(fmt.Sprintf("%f", val), "0")
		retVal = strings.Trim(retVal, ".")
	default:
		retVal = fmt.Sprintf("%v", val)
	}
	return retVal
}

func (s *Shedule) execTask(task SheduleTask) (err error, code int, response string) {
	params := make(req.Param)
	for key, val := range task.Request {
		s.lg.DebugF("[TaskExecutor %v] added parameter %v=%v to request", task.ID, key, val)
		params[key] = fmt.Sprintf("%v", wrapToValue(val))
	}
	s.lg.DebugF("[TaskExecutor %v] exec method %v", task.ID, task.Method)
	resp, err := req.Get(fmt.Sprintf("%v/%v", s.conf.ApiUrl, task.Method), params)
	jresp := ApiResponse{}
	if err != nil {
		return tracerr.Wrap(err), -1, ""
	} else if resp.Response().StatusCode != 200 {
		return fmt.Errorf("http err: %v - %v", resp.Response().StatusCode, resp.Response().Status), 0, ""
	} else if err := resp.ToJSON(&jresp); err != nil {
		return tracerr.Wrap(err), -1, ""
	} else if jresp.Code != 0 {
		return fmt.Errorf("%v", jresp.Error), jresp.Code, ""
	} else {
		respBody := ""
		if bytes, err := json.Marshal(jresp.Data); err != nil {
			return tracerr.Wrap(err), 0, fmt.Sprintf("Error decode :%v", string(resp.Bytes()))
		} else {
			respBody = string(bytes)
		}

		return nil, jresp.Code, respBody
	}
}

func (s *Shedule) sendTaskResponse(taskId int, code int, response string) error {
	respJson, err := json.Marshal(response)
	params := req.Param{
		"id":       taskId,
		"code":     code,
		"response": string(respJson),
	}
	resp, err := req.Get(fmt.Sprintf("%v/shedule/update", s.conf.ApiUrl), params)
	if err != nil {
		return tracerr.Wrap(err)
	} else if resp.Response().StatusCode != 200 {
		return tracerr.Wrap(fmt.Errorf("http err: %v - %v", resp.Response().StatusCode, resp.Response().Status))
	} else {
		return nil
	}
}
