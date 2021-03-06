package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-xorm/xorm"
	"github.com/nttdots/go-dots/dots_server/db_models"
	log "github.com/sirupsen/logrus"
)

/*
 * Stores ThroughputData to the throughput_data table in the DB.
 * if the ID of newly created object equals to 0, this function creates a new entry,
 * otherwise updates the relevant entry.
 * If it does not find any relevant entry, it just returns.
 */
func storeThroughputData(session *xorm.Session, td *ThroughputData) (err error) {
	if td == nil {
		return
	}

	dtp := db_models.ThroughputData{
		Id:  td.Id(),
		Pps: td.Pps(),
		Bps: td.Bps(),
	}

	if dtp.Id == 0 {
		_, err = session.Insert(&dtp)
		log.WithFields(log.Fields{
			"data":  dtp,
			"error": err,
		}).Debug("insert ThroughputData")
		if err != nil {
			return err
		}
		td.SetId(dtp.Id)
	} else {
		_, err = session.Where("id=?", dtp.Id).Cols("pps", "bps").Update(&dtp)
		log.WithFields(log.Fields{
			"data":  dtp,
			"error": err,
		}).Debug("update ThroughputData")
		if err != nil {
			return err
		}
	}

	return
}

/*
 * Obtains the protection status from the protection_status table by ID.
 * It also obtains relevant entries in the throughput_data table.
 *
 * If it does not find any entry by ID, it returns nil.
 */
func loadProtectionStatus(engine *xorm.Engine, id int64) (pps *ProtectionStatus, err error) {
	dps := db_models.ProtectionStatus{}

	protectionStatus := []db_models.ProtectionStatus{} // Todo: query(...).first()
	err = engine.Where("id=?", id).Find(&protectionStatus)
	//	ok, err := engine.ID(id).Get(&dps)
	if err != nil {
		return
	}
	/*
		if !ok {
			pps = nil
			return
		}
	*/

	/*
		peak, err := loadThroughput(engine, dps.PeakThroughputId)
		if err != nil {
			return
		}
		average, err := loadThroughput(engine, dps.AverageThroughputId)
		if err != nil {
			return
		}
	*/

	// skipping ThroughputData for now. will fix
	pps = NewProtectionStatus(
		//dps.Id, dps.TotalPackets, dps.TotalBits, peak, average,
		dps.Id, dps.TotalPackets, dps.TotalBits, &ThroughputData{0, 0, 0}, &ThroughputData{0, 0, 0},
	)
	return
}

/*
 * Obtains the throughput data by ID.
 * If there is no entry specified by the ID, it returns nil.
 */
func loadThroughput(engine *xorm.Engine, id int64) (ptd *ThroughputData, err error) {
	dtd := db_models.ThroughputData{}

	throughputData := []db_models.ThroughputData{}
	err = engine.Where("id=?", id).Find(&throughputData)
	//	ok, err := engine.ID(id).Get(&dtd)
	if err != nil {
		return
	}
	/*
		if !ok {
			ptd = nil
			return
		}
	*/
	dtd = throughputData[0]
	ptd = &ThroughputData{
		id:  dtd.Id,
		bps: dtd.Bps,
		pps: dtd.Pps,
	}

	return
}

/*
 * Stores a ProtectionStatus to the protection_status table in the DB.
 * if the ID of newly created object equals to 0, this function creates a new entry,
 * otherwise updates the relevant entry.
 * If it does not find any relevant entry, it just returns.
 */
func storeProtectionStatus(session *xorm.Session, ps *ProtectionStatus) (err error) {
	if ps == nil {
		return
	}

	peakId := ps.PeakThroughput().Id()
	// check if there is already an entry with this ID.
	if count, err := session.Where("id=?", peakId).Count(&db_models.ThroughputData{}); count == 0 || err != nil {
		peakId = 0
	}

	averageId := ps.AverageThroughput().Id()
	// check if there is already an entry with this ID.
	if count, err := session.Where("id=?", averageId).Count(&db_models.ThroughputData{}); count == 0 || err != nil {
		averageId = 0
	}

	dps := db_models.ProtectionStatus{
		Id:                  ps.Id(),
		TotalBits:           ps.TotalBits(),
		TotalPackets:        ps.TotalPackets(),
		PeakThroughputId:    peakId,
		AverageThroughputId: averageId,
	}

	if dps.Id == 0 {
		_, err = session.Insert(&dps)
		log.WithFields(log.Fields{
			"data": dps,
			"err":  err,
		}).Debug("insert ProtectionStatus")
		if err != nil {
			return
		}
		ps.SetId(dps.Id)
	} else {
		_, err = session.Where("id=?", dps.Id).Cols("total_bits", "total_packets", "peak_throughput_id", "averate_throughput_id").Update(&dps)
		log.WithFields(log.Fields{
			"data": dps,
			"err":  err,
		}).Debug("update ProtectionStatus")
		if err != nil {
			return
		}
	}

	return
}

/*
 * Stores a Protection object to the DB.
 *
 * parameter:
 *  protection ProtectionBase
 *  protectionStatus ProtectionStatus
 *  protectionParameters db_models.ProtectionParameter
 * return:
 *  err error
 */
func CreateProtection2(protection Protection) (newProtection db_models.Protection, err error) {

	var protectionParameters []db_models.ProtectionParameter
	var forwardedDataInfo, blockedDataInfo *db_models.ProtectionStatus
	var blockerId int64
	var storedProtection []db_models.Protection

	// database connection create
	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// transaction start
	session := engine.NewSession()
	defer session.Close()

	err = session.Begin()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("session create error")
		return
	}

	// inner function to store throughput data.
	storeThroughput := func(session *xorm.Session, data *ThroughputData, throughputId *int64, name string) (err error) {
		err = storeThroughputData(session, data)
		if err != nil {
			log.WithFields(log.Fields{
				"MitigationId": newProtection.MitigationId,
				"Pps":          data.Pps,
				"Bps":          data.Bps,
				"Err":          err,
			}).Errorf("insert %sThroughputData error", name)
			return
		}
		*throughputId = data.Id()
		return nil
	}

	// Registering ForwardedDataInfo Group
	if protection.ForwardedDataInfo() != nil {
		forwardedDataInfo = &db_models.ProtectionStatus{
			TotalPackets: protection.ForwardedDataInfo().TotalPackets(),
			TotalBits:    protection.ForwardedDataInfo().TotalBits(),
		}

		// Registered ThroughputData
		err = storeThroughput(session, protection.ForwardedDataInfo().PeakThroughput(), &forwardedDataInfo.PeakThroughputId, "ForwardedPeakThroughput")
		if err != nil {
			goto Rollback
		}
		err = storeThroughput(session, protection.ForwardedDataInfo().AverageThroughput(), &forwardedDataInfo.AverageThroughputId, "ForwardedAverageThroughput")
		if err != nil {
			goto Rollback
		}

		// Registered ForwardedDataInfo
		_, err = session.Insert(forwardedDataInfo)
		if err != nil {
			log.WithFields(log.Fields{
				"MitigationId":                  newProtection.MitigationId,
				"ForwardedDataInfoTotalPackets": forwardedDataInfo.TotalPackets,
				"ForwardedDataInfoTotalBits":    forwardedDataInfo.TotalBits,
				"Err": err,
			}).Error("insert ProtectionStatus error")
			goto Rollback
		}
	}

	// Registered BlockedDataInfo Group
	if protection.BlockedDataInfo() != nil {
		blockedDataInfo = &db_models.ProtectionStatus{
			TotalPackets: protection.BlockedDataInfo().TotalPackets(),
			TotalBits:    protection.BlockedDataInfo().TotalBits(),
		}

		// Registered ThroughputData
		err = storeThroughput(session, protection.BlockedDataInfo().PeakThroughput(), &blockedDataInfo.PeakThroughputId, "BlockedPeakThroughput")
		if err != nil {
			goto Rollback
		}
		err = storeThroughput(session, protection.BlockedDataInfo().AverageThroughput(), &blockedDataInfo.AverageThroughputId, "BlockedAverageThroughput")
		if err != nil {
			goto Rollback
		}

		// Registered BlockedDataInfo
		_, err = session.Insert(blockedDataInfo)
		if err != nil {
			log.WithFields(log.Fields{
				"MitigationId":                newProtection.MitigationId,
				"BlockedDataInfoTotalPackets": blockedDataInfo.TotalPackets,
				"BlockedDataInfoTotalBits":    blockedDataInfo.TotalBits,
				"Err": err,
			}).Error("insert ProtectionStatus error")
			goto Rollback
		}
	}

	if protection.TargetBlocker() != nil {
		blockerId = protection.TargetBlocker().Id()
	}

	// Registered protection
	newProtection = db_models.Protection{
		Type:                string(protection.Type()),
		MitigationId:        protection.MitigationId(),
		TargetBlockerId:     blockerId,
		IsEnabled:           protection.IsEnabled(),
		StartedAt:           protection.StartedAt(),
		FinishedAt:          protection.FinishedAt(),
		RecordTime:          protection.RecordTime(),
		ForwardedDataInfoId: forwardedDataInfo.Id,
		BlockedDataInfoId:   blockedDataInfo.Id,
	}
	_, err = session.Insert(&newProtection)
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Info("protection insert")
		goto Rollback
	}

	err = session.Commit()
	session = engine.NewSession()
	defer session.Close()

	// obtain the new protection stored in the DB for update the id.
	storedProtection = make([]db_models.Protection, 0)
	if err := engine.Where("mitigation_id = ?", newProtection.MitigationId).Find(&storedProtection); err != nil {
		goto Rollback
	}

	log.WithFields(log.Fields{
		"protection": newProtection,
	}).Debug("create new protection")

	// Registering ProtectionParameters
	protectionParameters = toProtectionParameters(protection, storedProtection[0].Id)
	if len(protectionParameters) > 0 {
		/*
			for idx := range protectionParameters {
				protectionParameters[idx].ProtectionId = storedProtection[0].Id
			}
		*/

		_, err = session.InsertMulti(protectionParameters)
		if err != nil {
			log.WithFields(log.Fields{
				"MitigationId": newProtection.MitigationId,
				"Err":          err,
			}).Error("insert ProtectionParameter error")
			goto Rollback
		}
		log.WithFields(log.Fields{
			"protection_parameters": protectionParameters,
		}).Debug("create new protection_parameter")
	}

	// add Commit() after all actions
	err = session.Commit()

	// obtain the new protection stored in the DB for update the id.
	/*
		storedProtection = make([]db_models.Protection, 0)
		if err := engine.Where("mitigation_id = ?", newProtection.MitigationId).Find(&storedProtection); err == nil {
			return storedProtection[0], nil
		}
	*/

	return storedProtection[0], nil

Rollback:
	session.Rollback()
	return
}

/*
 * Updates a Protection object in the DB.
 *
 * parameter:
 *  protection ProtectionBase
 * return:
 *  err error
 */
func UpdateProtection(protection Protection) (err error) {
	var protectionParameters []db_models.ProtectionParameter
	var updProtection *db_models.Protection
	var updForwardedDataInfo, updBlockedDataInfo *db_models.ProtectionStatus
	var chk bool

	// database connection create
	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// transaction start
	session := engine.NewSession()
	defer session.Close()

	err = session.Begin()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("session create error")
		return
	}

	// inner function to update throughput data.
	updateThroughput := func(session *xorm.Session, data *ThroughputData, name string) (err error) {
		err = storeThroughputData(session, data)
		if err != nil {
			log.WithFields(log.Fields{
				"MitigationId": protection.MitigationId,
				"Pps":          data.Pps,
				"Bps":          data.Bps,
				"Err":          err,
			}).Errorf("update %sThroughputData error", name)
			return
		}
		return nil
	}

	// Updated protection
	updProtection = &db_models.Protection{}
	chk, err = session.Where("id=?", protection.Id()).Get(updProtection)

	if err != nil {
		log.WithFields(log.Fields{
			"id":           protection.Id(),
			"MitigationId": protection.MitigationId(),
			"Err":          err,
		}).Error("select Protection error")
		goto Rollback
	}
	if !chk {
		// no data found
		log.WithFields(log.Fields{
			"id":           protection.Id(),
			"MitigationId": protection.MitigationId(),
		}).Info("update Protection data not exist.")
		goto Rollback
	}

	updProtection.IsEnabled = protection.IsEnabled()
	updProtection.StartedAt = protection.StartedAt()
	updProtection.FinishedAt = protection.FinishedAt()
	updProtection.RecordTime = protection.RecordTime()
	_, err = session.Where("id=?", updProtection.Id).Cols("is_enabled", "started_at", "finished_at", "record_time").Update(updProtection)

	if err != nil {
		log.WithFields(log.Fields{
			"ProtectionId": updProtection.Id,
			"Err":          err,
		}).Error("update Protection error")
		goto Rollback
	}

	// Updated ForwardedDataInfo Group
	if protection.ForwardedDataInfo() != nil {
		// Updated ThroughputData
		err = updateThroughput(session, protection.ForwardedDataInfo().PeakThroughput(), "ForwardedPeak")
		if err != nil {
			goto Rollback
		}
		err = updateThroughput(session, protection.ForwardedDataInfo().AverageThroughput(), "ForwardedAverage")
		if err != nil {
			goto Rollback
		}

		// Updated ForwardedDataInfo
		err = storeProtectionStatus(session, protection.ForwardedDataInfo())
		if err != nil {
			log.WithFields(log.Fields{
				"MitigationId":                  updProtection.MitigationId,
				"ForwardedDataInfoTotalPackets": updForwardedDataInfo.TotalPackets,
				"ForwardedDataInfoTotalBits":    updForwardedDataInfo.TotalBits,
				"Err": err,
			}).Error("update ProtectionStatus error")
			goto Rollback
		}
	}

	if protection.BlockedDataInfo() != nil {
		// Updated ThroughputData
		err = updateThroughput(session, protection.BlockedDataInfo().PeakThroughput(), "BlockedPeak")
		if err != nil {
			goto Rollback
		}
		err = updateThroughput(session, protection.BlockedDataInfo().AverageThroughput(), "BlockedAverage")
		if err != nil {
			goto Rollback
		}

		// Updated BlockedDataInfo
		err = storeProtectionStatus(session, protection.BlockedDataInfo())
		if err != nil {
			log.WithFields(log.Fields{
				"MitigationId":                updProtection.MitigationId,
				"BlockedDataInfoTotalPackets": updBlockedDataInfo.TotalPackets,
				"BlockedDataInfoTotalBits":    updBlockedDataInfo.TotalBits,
				"Err": err,
			}).Error("update ProtectionStatus error")
			goto Rollback
		}
	}

	// Update ProtectionParameters
	// delete the existing entry with the same id.
	_, err = session.Delete(&db_models.ProtectionParameter{ProtectionId: protection.Id()})
	if err != nil {
		log.WithFields(log.Fields{
			"ProtectionId": updProtection.Id,
			"MitigationId": updProtection.MitigationId,
			"Err":          err,
		}).Error("delete ParameterValue error")
		goto Rollback
	}

	protectionParameters = toProtectionParameters(protection, protection.Id())

	if len(protectionParameters) > 0 {
		_, err = session.InsertMulti(&protectionParameters)
		if err != nil {
			log.WithFields(log.Fields{
				"ProtectionId": updProtection.Id,
				"MitigationId": updProtection.MitigationId,
				"Err":          err,
			}).Error("insert ParameterValue error")
			goto Rollback
		}
	}

	// add Commit() after all actions
	err = session.Commit()
	return
Rollback:
	session.Rollback()
	return
}

/*

 */
func GetProtectionByMitigationId(mitigationId int, companyId int) (p Protection, err error) {

	engine, err := ConnectDB()
	if err != nil {
		log.Errorf("database connect error: %s", err)
		return
	}

	var ps []db_models.Protection
	err = engine.Where("mitigation_id = ?", mitigationId).Find(&ps)
	if err != nil {
		return nil, err
	}

	if len(ps) == 0 {
		return nil, nil
	}

	if len(ps) != 1 {
		return nil, errors.New("duplicate mitigationId.")
	}

	return toProtection(engine, ps[0])
}

func GetProtectionById(id int64) (p Protection, err error) {
	dbp := db_models.Protection{}

	engine, err := ConnectDB()
	if err != nil {
		log.Errorf("database connect error: %s", err)
		return
	}

	// FIXME: codes below does not work properly... maybe aroud the database schema
	//ok, err := engine.Id(id).Get(&dbp)
	protections := []db_models.Protection{} // Todo: query(...).first()
	err = engine.Where("id=?", id).Find(&protections)
	if err != nil {
		log.WithField("id", id).Warnf("GetProtectionById: protection not found.", err)
		return nil, nil
	}
	dbp = protections[0]

	p, err = toProtection(engine, dbp)

	return
}

/*
 * Converts db_models.Protection to models.Protection.
 */
func toProtection(engine *xorm.Engine, dbp db_models.Protection) (p Protection, err error) {

	forwardDataInfo, err := loadProtectionStatus(engine, dbp.ForwardedDataInfoId)
	if err != nil {
		return
	}
	blockedDataInfo, err := loadProtectionStatus(engine, dbp.BlockedDataInfoId)
	if err != nil {
		return
	}

	var blocker Blocker
	if dbp.TargetBlockerId == 0 {
		blocker = nil
	} else {
		// var dbl db_models.Blocker
		// ok, err := engine.ID(dbp.TargetBlockerId).Get(&dbl)
		blockers := []db_models.Blocker{} // Todo: query(...).first()
		err = engine.Where("id=?", dbp.TargetBlockerId).Find(&blockers)
		if err != nil {
			return nil, err
		}
		if len(blockers) == 0 {
			blocker = nil
		}
		dbl := blockers[0]
		/*
			if !ok {
				blocker = nil
			}
		*/
		blocker, err = toBlocker(dbl)
		if err != nil {
			return nil, err
		}
	}

	pb := ProtectionBase{
		dbp.Id,
		dbp.MitigationId,
		blocker,
		dbp.IsEnabled,
		dbp.StartedAt,
		dbp.FinishedAt,
		dbp.RecordTime,
		forwardDataInfo,
		blockedDataInfo,
	}

	var params []db_models.ProtectionParameter
	err = engine.Where("protection_id = ?", dbp.Id).Find(&params)
	if err != nil {
		return nil, err
	}

	parametersMap := ProtectionParametersToMap(params)
	switch dbp.Type {
	case PROTECTION_TYPE_RTBH:
		p = NewRTBHProtection(pb, parametersMap)
	case PROTECTION_TYPE_BLACKHOLE:
		p = NewBlackHoleProtection(pb, parametersMap)
	default:
		p = nil
		err = errors.New(fmt.Sprintf("invalid protection type: %s", dbp.Type))
	}

	return

}

/*
 * Obtains a Protection object by ID.
 *
 * parameter:
 *  mitigationId mitigation ID
 * return:
 *  protection Protection
 *  error error
 */
func GetProtectionBase(mitigationId int) (protection ProtectionBase, err error) {
	// default value setting
	dbProtection := db_models.Protection{}

	// database connection create
	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// Get protection
	ok, err := engine.Where("mitigation_id = ?", mitigationId).Get(&dbProtection)
	if err != nil {
		log.WithFields(log.Fields{
			"MitigationId": mitigationId,
			"Err":          err,
		}).Error("select Protection error")
		return
	}
	if !ok {
		return ProtectionBase{}, nil
	}

	forwardInfo, err := loadProtectionStatus(engine, dbProtection.ForwardedDataInfoId)
	if err != nil {
		log.WithFields(log.Fields{
			"MitigationId": mitigationId,
			"Err":          err,
		}).Error("load forwarded_data_info error")
		return
	}
	blockedInfo, err := loadProtectionStatus(engine, dbProtection.BlockedDataInfoId)
	if err != nil {
		log.WithFields(log.Fields{
			"MitigationId": mitigationId,
			"Err":          err,
		}).Error("load blocked_data_info error")
		return
	}

	var blocker Blocker
	if dbProtection.TargetBlockerId == 0 {
		blocker = nil
	} else {
		b, err := GetBlockerById(dbProtection.TargetBlockerId)
		if err != nil {
			return ProtectionBase{}, err
		}
		blocker, err = toBlocker(b)
		if err != nil {
			return ProtectionBase{}, err
		}
	}

	// from db_models to models
	protection = ProtectionBase{
		id:                dbProtection.Id,
		mitigationId:      dbProtection.MitigationId,
		targetBlocker:     blocker,
		isEnabled:         dbProtection.IsEnabled,
		startedAt:         dbProtection.StartedAt,
		finishedAt:        dbProtection.FinishedAt,
		recordTime:        dbProtection.RecordTime,
		forwardedDataInfo: forwardInfo,
		blockedDataInfo:   blockedInfo,
	}

	return
}

/*
 * Obtains a list of ProtectionParameter objects by ID of the parent Protection.
 *
 * parameter:
 *  protectionId id of the parent Protection
 * return:
 *  ProtectionParameter []db_models.ProtectionParameter
 *  error error
 */
func GetProtectionParameters(protectionId int64) (protectionParameters []db_models.ProtectionParameter, err error) {
	// default value setting
	protectionParameters = []db_models.ProtectionParameter{}

	// database connection create
	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// Get protection_parameters table data
	err = engine.Where("protection_id = ?", protectionId).Find(&protectionParameters)
	if err != nil {
		log.WithFields(log.Fields{
			"ProtectionId": protectionId,
			"Err":          err,
		}).Error("select Protection error")
		return
	}

	return

}

/*
 * Deletes a protection and the related ProtectionStatus and ThroughputData.
 */
func deleteProtection(session *xorm.Session, protection db_models.Protection) (err error) {

	forwardInfo := db_models.ProtectionStatus{}
	ok, err := session.Where("id=?", protection.ForwardedDataInfoId).Get(&forwardInfo)
	if ok {
		err = deleteProtectionStatus(session, forwardInfo)
		if err != nil {
			return
		}
	}

	blockedInfo := db_models.ProtectionStatus{}
	ok, err = session.Where("id=?", protection.BlockedDataInfoId).Get(&blockedInfo)
	if ok {
		err = deleteProtectionStatus(session, blockedInfo)
		if err != nil {
			return
		}
	}

	_, err = session.Where("protection_id = ?", protection.Id).Delete(db_models.ProtectionParameter{})
	if err != nil {
		return
	}

	_, err = session.Delete(db_models.Protection{Id: protection.Id})
	return
}

/*
 * Deletes a protection_status and the related ThroughputData.
 */
func deleteProtectionStatus(session *xorm.Session, status db_models.ProtectionStatus) (err error) {

	_, err = session.In("id", status.PeakThroughputId, status.AverageThroughputId).Delete(db_models.ThroughputData{})
	if err != nil {
		return
	}

	_, err = session.Delete(db_models.ProtectionStatus{Id: status.Id})
	return
}

func DeleteProtectionById(id int64) (err error) {

	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// transaction start
	session := engine.NewSession()
	defer session.Close()

	err = session.Begin()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("session create error")
		return
	}

	p := db_models.Protection{}
	ok, err := session.Where("id=?", id).Get(&p)
	if err != nil {
		goto Error
	}
	if !ok {
		// already deleted
		goto Error
	}

	err = deleteProtection(session, p)
	if err != nil {
		goto Error
	}

	session.Commit()
	return

Error:
	session.Rollback()
	return
}

/*
 * Deletes a protection by the mitigation ID.
 *
 * parameter:
 *  mitigationId mitigation ID
 * return:
 *  error error
 */
func DeleteProtection(mitigationId int) (err error) {
	var protection db_models.Protection
	var chk bool

	// database connection create
	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// transaction start
	session := engine.NewSession()
	defer session.Close()

	err = session.Begin()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("session create error")
		return
	}

	// Get Protection
	protection = db_models.Protection{}
	chk, err = session.Where("mitigation_id = ?", mitigationId).Get(&protection)
	if err != nil {
		log.WithFields(log.Fields{
			"MitigationId": mitigationId,
			"Err":          err,
		}).Error("select Protection error")
		goto Rollback
	}
	if !chk {
		// no data found
		goto Rollback
	}

	err = deleteProtection(session, protection)
	if err != nil {
		log.WithFields(log.Fields{
			"Id":  protection.Id,
			"Err": err,
		}).Error("delete Protection error")
		goto Rollback
	}

	session.Commit()
	return
Rollback:
	session.Rollback()
	return
}

func ProtectionParametersToMap(params []db_models.ProtectionParameter) map[string][]string {
	m := make(map[string][]string)

	for _, p := range params {
		a, ok := m[p.Key]
		if !ok {
			a = make([]string, 0)
		}
		a = append(a, p.Value)
		m[p.Key] = a
	}
	return m
}

func ProtectionStatusToDbModel(protectionId int64, status *ProtectionStatus) (newProtectionStatus db_models.ProtectionStatus) {
	newProtectionStatus = db_models.ProtectionStatus{
		Id:           protectionId,
		TotalPackets: status.TotalPackets(),
		TotalBits:    status.TotalBits(),
	}

	return
}

/*
 * Updates a protection object in the DB on the invocation of the protection.
 *  1. turn the is_enabled field on and updates the started_at with current datetime.
 *  2. increase the value in the load field of the blocker.
 */
func StartProtection(p Protection, b Blocker) (err error) {
	dbb := db_models.Blocker{}

	// database connection create
	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// transaction start
	session := engine.NewSession()
	defer session.Close()

	start := time.Now()

	// protection is_enabled -> true, start_at -> now
	count, err := session.Where("id=?", p.Id()).Cols("is_enabled", "started_at").Update(&db_models.Protection{IsEnabled: true, StartedAt: start})
	log.WithFields(
		log.Fields{"id": p.Id(), "blockerId": b.Id(), "count": count},
	).WithError(err).Debug("update protection. is_enable -> true, start_at -> now")
	if count != 1 || err != nil {
		goto ROLLBACK
	}

	// blocker load + 1
	count, err = session.Where("id=?", b.Id()).Incr("load", 1).Update(&dbb)
	log.WithFields(
		log.Fields{"id": p.Id(), "count": count},
	).WithError(err).Debug("update blocker. load = load + 1")
	if count != 1 || err != nil {
		goto ROLLBACK
	}
	_, err = session.Where("id = ?", b.Id()).Get(&dbb)
	if err != nil {
		goto ROLLBACK
	}

	session.Commit()

	p.SetStartedAt(start)
	p.SetIsEnabled(true)
	b.SetLoad(dbb.Load)

	return
ROLLBACK:
	session.Rollback()
	return
}

/*
 * Updates a protection object in the DB on the termination of the protection.
 *  1. turn the is_enabled field off and updates the finished_at with current datetime.
 *  2. decrease the value in the load field of the blocker.
 */
func StopProtection(p Protection, b Blocker) (err error) {
	// database connection create
	engine, err := ConnectDB()
	if err != nil {
		log.WithFields(log.Fields{
			"Err": err,
		}).Error("database connect error")
		return
	}

	// transaction start
	session := engine.NewSession()
	defer session.Close()

	// protection is_enabled -> true, start_at -> now
	count, err := session.Where("id = ?", p.Id()).Cols("is_enabled", "finished_at").Update(&db_models.Protection{IsEnabled: false, FinishedAt: time.Now()})
	log.WithFields(
		log.Fields{"id": p.Id(), "count": count},
	).WithError(err).Debug("update protection. is_enable -> false, finished_at -> now")
	if count != 1 || err != nil {
		goto ROLLBACK
	}

	// blocker load - 1
	count, err = session.Where("id = ?", b.Id()).Incr("load", -1).Update(&db_models.Blocker{})
	log.WithFields(
		log.Fields{"id": p.Id(), "count": count},
	).WithError(err).Debug("update blocker. load = load - 1")
	if count != 1 || err != nil {
		goto ROLLBACK
	}

	session.Commit()
	return
ROLLBACK:
	session.Rollback()
	return
}
