package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	pb "github.com/secretflow/scql/pkg/proto-gen/scql"
	"github.com/secretflow/scql/pkg/util/brokerutil"
	"github.com/secretflow/scql/pkg/util/tableview"
	log "github.com/sirupsen/logrus"
)

var brokerCommand *brokerutil.Command

// func init() {
// 	brokerCommand = brokerutil.NewCommand("http://127.0.0.1:8080", 5)
// }

var projectConf = `{"spu_runtime_cfg":{"protocol":"SEMI2K","field":"FM64"},"session_expire_seconds":"86400"}`
var projectID = "tsql"

func createProject() error {
	_, err := brokerCommand.CreateProject(projectID, projectConf)
	if err != nil {
		return err
	}
	fmt.Println("create project successed")
	return nil
}

func inviteMember(member string) error {
	err := brokerCommand.InviteMember(projectID, member)
	if err != nil {
		return err
	}
	fmt.Printf("invite %v succeeded\n", member)
	return nil
}

// 检查成员 member 是否加入项目
func ProjectMemberJoined(member string) (bool, error) {
	response, err := brokerCommand.GetProject(projectID)
	if err != nil {
		return false, err
	}
	if len(response.GetProjects()) == 0 {
		return false, errors.New("not get project")
	}
	project := response.GetProjects()[0]
	for _, mem := range project.Members {
		if mem == member {
			return true, nil
		}
	}
	return false, nil
}

// 查看邀请--同意
func JoinProject() (bool, error) {
	response, err := brokerCommand.GetInvitation()
	if err != nil {
		return false, err
	}
	if len(response.Invitations) > 0 {
		invitation := response.Invitations[0]
		err = processInvitation(fmt.Sprintf("%d", invitation.InvitationId))
		if err != nil {
			return false, err
		} else {
			return true, nil
		}
	} else {
		return false, nil
	}
}

// 同意邀请
func processInvitation(ids string) error {
	accept := true
	err := brokerCommand.ProcessInvitation(ids, accept)
	if err != nil {
		return err
	}
	log.Printf("process invitation %v succeeded\n", ids)
	return nil
}

func createTable(req *RunPrivacyRequest) error {
	var columnDescs []*pb.CreateTableRequest_ColumnDesc
	for _, column := range req.Columns {
		columnDescs = append(columnDescs, &pb.CreateTableRequest_ColumnDesc{
			Name:  column.Column,
			Dtype: column.Type,
		})
	}
	err := brokerCommand.CreateTable(projectID, req.User, "mysql", "engine."+req.User, columnDescs)
	if err != nil {
		log.Debug(err)
		return err
	}
	log.Debug("create table succeeded")
	return nil
}

func grantCCL(party, tableName, colName, constraint string) error {
	value, ok := pb.Constraint_value[constraint]
	if !ok {
		return fmt.Errorf("not support constraint %v", constraint)
	}
	var ccls []*pb.ColumnControl
	ccls = append(ccls, &pb.ColumnControl{
		Col: &pb.ColumnDef{
			ColumnName: colName,
			TableName:  tableName,
		},
		PartyCode:  party,
		Constraint: pb.Constraint(value),
	})

	err := brokerCommand.GrantCCL(projectID, ccls)
	if err != nil {
		return err
	}
	fmt.Println("grant ccl succeeded")
	return nil
}

func runQuery(query, filename string) error {
	response, err := brokerCommand.DoQuery(projectID, query, &pb.DebugOptions{EnablePsiDetailLog: false}, "{}")
	if err != nil {
		return err
	}
	if len(response.Result.OutColumns) > 0 {
		f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		table := tablewriter.NewWriter(f)
		table.SetAutoWrapText(false)
		table.SetAutoFormatHeaders(false)
		if err := tableview.ConvertToTable(response.Result.GetOutColumns(), table); err != nil {
			log.Debugf("[fetch]convertToTable with err:%v\n", err)
		}
		log.Debugf("%v rows in set: (%vs)\n", table.NumLines(), response.Result.GetCostTimeS())
		table.Render()
	}
	log.Debug("run query succeeded")
	return nil
}
