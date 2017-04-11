package app

import (
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"

	"github.com/fasthall/gochariots/info"
	"github.com/fasthall/gochariots/maintainer"
	"github.com/fasthall/gochariots/record"
	"github.com/gin-gonic/gin"
)

var batcherConn []net.Conn
var batcherPool []string

type JsonRecord struct {
	Tags    map[string]string `json:"tags"`
	PreHost int               `json:"prehost"`
	PreTOId int               `json:"pretoid"`
}

func Run(port string) {
	router := gin.Default()
	router.POST("/record", postRecord)
	router.GET("/record/:lid", getRecord)
	router.POST("/batcher", addBatcher)
	router.GET("/batcher", getBatchers)
	router.Run(":" + port)
}

func addBatcher(c *gin.Context) {
	batcherPool = append(batcherPool, c.Query("host"))
	batcherConn = make([]net.Conn, len(batcherPool))
	c.String(http.StatusOK, c.Query("host")+" added\n")
}

func getBatchers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"batchers": batcherPool,
	})
}

func dialConn(hostID int) error {
	var err error
	batcherConn[hostID], err = net.Dial("tcp", batcherPool[hostID])
	return err
}

func postRecord(c *gin.Context) {
	var jsonRecord JsonRecord
	err := c.Bind(&jsonRecord)
	if err != nil {
		panic(err)
	}

	// send to batcher
	r := record.Record{
		Host: info.ID,
		Tags: jsonRecord.Tags,
		Pre: record.Causality{
			Host: jsonRecord.PreHost,
			TOId: jsonRecord.PreTOId,
		},
	}
	b := []byte{'r'}
	jsonBytes, err := record.ToJSON(r)
	if err != nil {
		panic(err)
	}

	hostID := rand.Intn(len(batcherPool))
	if batcherConn[hostID] == nil {
		err = dialConn(hostID)
		if err != nil {
			log.Printf("%s couldn't connect to the batcherPool[%d] %s", info.GetName(), hostID, batcherPool[hostID])
			c.String(http.StatusServiceUnavailable, "Couldn't connect to the batcher")
			return
		}
	}
	cnt := 5
	sent := false
	for sent == false {
		_, err = batcherConn[hostID].Write(append(b, jsonBytes...))
		if err != nil {
			if cnt >= 0 {
				cnt--
				err = dialConn(hostID)
				if err != nil {
					log.Printf("%s couldn't connect to the batcherPool[%d] %s, retrying...", info.GetName(), hostID, batcherPool[hostID])
				}
			} else {
				log.Printf("%s failed to connect to the batcherPool[%d] %s after retrying 5 times", info.GetName(), hostID, batcherPool[hostID])
				c.String(http.StatusServiceUnavailable, "Couldn't connect to the batcher")
				return
			}
		} else {
			sent = true
		}
	}
}

func getRecord(c *gin.Context) {
	lid, err := strconv.Atoi(c.Param("lid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": "Invalid LId",
			"error":   err,
		})
		return
	}
	record, err := maintainer.ReadByLId(lid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "Record not found",
			"error":   err,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"LId":       record.LId,
		"Host":      record.Host,
		"TOId":      record.TOId,
		"Causality": record.Pre,
		"Tags":      record.Tags,
	})
}
