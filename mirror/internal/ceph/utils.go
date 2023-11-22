package ceph

import (
	"10.1.1.220/cdm/cdm-cloud/common/errors"
	"10.1.1.220/cdm/cdm-cloud/common/logger"
	"10.1.1.220/cdm/cdm-disaster-recovery/common/mirror"
	"10.1.1.220/cdm/cdm-disaster-recovery/daemons/mirror/internal"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

func execCommand(c string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", c)
	b, err := cmd.Output()

	logger.Infof("[execCommand] command: sh -c %s", c)

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return nil, TimeoutExecCommand(c)

	case err != nil:
		if ee, ok := err.(*exec.ExitError); ok {
			b = ee.Stderr
		}
		return nil, UnexpectedExecResult(string(b), err)
	}
	return b, nil
}

func rbdPoolPeerInfoCommand(p, k, c string) string {
	k = strings.ReplaceAll(k, "/", ".")
	return fmt.Sprintf("rbd mirror pool info -p %s --conf %s/%s.conf --cluster %s -n client.%s --format json", p, DefaultCephPath, k, k, c)
}

func rbdPoolEnableCommand(p, k, c string) string {
	k = strings.ReplaceAll(k, "/", ".")
	return fmt.Sprintf("rbd mirror pool enable image -p %s --conf %s/%s.conf --cluster %s -n client.%s", p, DefaultCephPath, k, k, c)
}

func rbdPoolDisableCommand(p, k, c string) string {
	k = strings.ReplaceAll(k, "/", ".")
	return fmt.Sprintf("rbd mirror pool disable image -p %s --conf %s/%s.conf --cluster %s -n client.%s", p, DefaultCephPath, k, k, c)
}

func rbdPoolPeerRemoveCommand(p, k, c, uuid string) string {
	k = strings.ReplaceAll(k, "/", ".")
	return fmt.Sprintf("rbd mirror pool peer remove -p %s %s --conf %s/%s.conf --cluster %s -n client.%s", p, uuid, DefaultCephPath, k, k, c)
}

func rbdPoolPeerAddCommand(p, sc, sk, tk, tc string) string {
	tk = strings.ReplaceAll(tk, "/", ".")
	sk = strings.ReplaceAll(sk, "/", ".")
	return fmt.Sprintf("rbd mirror pool peer add -p %s client.%s@%s --conf %s/%s.conf --cluster %s -n client.%s", p, sc, sk, DefaultCephPath, tk, tk, tc)
}

func rbdPoolImageListCommand(pool, key, client string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd -p %s ls --conf %s/%s.conf -n client.%s --format json", pool, DefaultCephPath, key, client)
}

func rbdImageInfoCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd info -p %s %s --conf %s/%s.conf -n client.%s --format json", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageEnableCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd mirror image enable -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageDisableCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd mirror image disable -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageStatusCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd mirror image status -p %s %s --conf %s/%s.conf -n client.%s --format json", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImagePromoteCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd mirror image promote --force -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageDemoteCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd mirror image demote -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageReSyncCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd mirror image resync -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageFeatureDisableJournaling(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd feature disable -p %s %s journaling --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageFeatureEnableJournaling(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd feature enable -p %s %s journaling --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdClientKeyring(c, k string) string {
	return fmt.Sprintf("[client.%s]\n\tkey = %s\n", c, k)
}

func rbdClientKeyringPath(s, c string) string {
	return strings.ReplaceAll(fmt.Sprintf("%s.client.%s.keyring", s, c), "/", ".")
}

func rbdMirrorImageSnapshotPurgeCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd snap purge -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageObjectMapRebuild(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd object-map rebuild -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageDeleteCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd remove -p %s %s --conf %s/%s.conf -n client.%s", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageSnapshotListCommand(pool, key, client, volume string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd snap ls -p %s %s --conf %s/%s.conf -n client.%s --format json", pool, volume, DefaultCephPath, key, client)
}

func rbdMirrorImageUnprotectSnapshotCommand(pool, key, client, volume, snapshot string) string {
	key = strings.ReplaceAll(key, "/", ".")
	return fmt.Sprintf("rbd snap unprotect -p %s %s@%s --conf %s/%s.conf -n client.%s", pool, volume, snapshot, DefaultCephPath, key, client)
}

func rbdConfPath(s string) string {
	return strings.ReplaceAll(fmt.Sprintf("%s.conf", s), "/", ".")
}

// RbdMirrorRunCommand rbd-mirror 실행 command
func RbdMirrorRunCommand(k, c string) string {
	k = strings.ReplaceAll(k, "/", ".")
	return fmt.Sprintf("rbd-mirror --conf %s/%s.conf --id %s -d", DefaultCephPath, k, c)
}

func getRBDMirrorPoolInfo(p, k, c string) (*peerInfoResult, error) {
	logger.Infof("[getRBDMirrorPoolInfo] exec rbdPoolPeerInfoCommand")
	b, err := execCommand(rbdPoolPeerInfoCommand(p, k, c))
	if err != nil {
		logger.Errorf("[getRBDMirrorPoolInfo] Could not exec command: rbdPoolPeerInfoCommand. Cause: %+v", err)
		return nil, err
	}

	var r peerInfoResult
	if err = json.Unmarshal(b, &r); err != nil {
		logger.Errorf("[getRBDMirrorPoolInfo] Could not unmarshal: peerInfoResult. Cause: %+v", err)
		return nil, errors.Unknown(err)
	}

	return &r, nil
}

func rbdConfigWrite(path string, b []byte) error {
	return ioutil.WriteFile(path, b, 0644)
}

func getCephClient(md map[string]interface{}) string {
	if _, ok := md["admin_client"]; ok {
		return md["admin_client"].(string)
	}
	return md["client"].(string)
}

// CreateRBDConfigFile ceph config 생성 함수
func CreateRBDConfigFile(k string, md map[string]interface{}) error {
	var err error
	filePath := path.Join(DefaultCephPath, rbdClientKeyringPath(k, md["client"].(string)))
	keyring := rbdClientKeyring(md["client"].(string), md["keyring"].(string))
	if err = rbdConfigWrite(filePath, []byte(keyring)); err != nil {
		logger.Errorf("[CreateRBDConfigFile] Could not write rbdConfig(client): ptah(%s) keyring(%s)", filePath, keyring)
		return errors.Unknown(err)
	}
	logger.Infof("[CreateRBDConfigFile] Done - write rbdConfig(client): ptah(%s) keyring(%s)", filePath, keyring)

	if val, ok := md["admin_client"]; ok {
		filePath = path.Join(DefaultCephPath, rbdClientKeyringPath(k, val.(string)))
		keyring = rbdClientKeyring(val.(string), md["admin_keyring"].(string))
		if err = rbdConfigWrite(filePath, []byte(keyring)); err != nil {
			logger.Errorf("[CreateRBDConfigFile] Could not write rbdConfig(admin_client): ptah(%s) keyring(%s)", filePath, keyring)
			return errors.Unknown(err)
		}
		logger.Infof("[CreateRBDConfigFile] Done - write rbdConfig(admin_client): ptah(%s) keyring(%s)", filePath, keyring)
	} else {
		logger.Warnf("[CreateRBDConfigFile] admin_client keyring is not existed")
	}

	filePath = path.Join(DefaultCephPath, rbdConfPath(k))
	if err = rbdConfigWrite(filePath, []byte(md["conf"].(string))); err != nil {
		logger.Errorf("[CreateRBDConfigFile] Could not write rbdConfig(conf): ptah(%s) conf(%s)", filePath, md["conf"].(string))
		return errors.Unknown(err)
	}
	logger.Infof("[CreateRBDConfigFile] Done - write rbdConfig: ptah(%s) conf(%s)", filePath, md["conf"].(string))

	return nil
}

// DeleteRBDConfig ceph config 삭제 함수
func DeleteRBDConfig(k string, md map[string]interface{}) error {
	logger.Infof("[DeleteRBDConfig] Start: key(%s)", k)

	var files = []string{path.Join(DefaultCephPath, rbdClientKeyringPath(k, md["client"].(string))), path.Join(DefaultCephPath, rbdConfPath(k))}

	if val, ok := md["admin_client"]; ok {
		files = append(files, path.Join(DefaultCephPath, rbdClientKeyringPath(k, val.(string))))
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			logger.Errorf("[DeleteRBDConfig] Could not delete file or not exist: file(%s). Cause: %+v", file, err)
			return errors.Unknown(err)
		}
		logger.Infof("[DeleteRBDConfig] Done - remove file(%s): key(%s)", file, k)
	}

	logger.Infof("[DeleteRBDConfig] Success: key(%s)", k)
	return nil
}

// RunRBDMirrorPeerAddProcess source, target mirror pool 확인 및 등록 함수
func RunRBDMirrorPeerAddProcess(sourceKey, targetKey string, source, target map[string]interface{}) error {
	logger.Infof("[RunRBDMirrorPeerAddProcess] Start: source(%s) target(%s)", sourceKey, targetKey)

	var err error
	var p *peerInfoResult

	if p, err = getRBDMirrorPoolInfo(target["pool"].(string), targetKey, getCephClient(target)); err != nil {
		return err
	}

	if p.Mode != "image" {
		logger.Infof("[RunRBDMirrorPeerAddProcess] peer info: mode(%s)", p.Mode)
		if _, err = execCommand(rbdPoolEnableCommand(target["pool"].(string), targetKey, getCephClient(target))); err != nil {
			logger.Errorf("[RunRBDMirrorPeerAddProcess] Could not exec command: rbdPoolEnableCommand. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[RunRBDMirrorPeerAddProcess] len(peer): %d", len(p.Peers))

	if len(p.Peers) == 0 {
		if _, err = execCommand(rbdPoolPeerAddCommand(target["pool"].(string), source["client"].(string), sourceKey, targetKey, getCephClient(target))); err != nil {
			logger.Errorf("[RunRBDMirrorPeerAddProcess] Could not exec command: rbdPoolPeerAddCommand. Cause: %+v", err)
			return err
		}
	} else if p.Peers[0].ClientName != "client."+source["client"].(string) || p.Peers[0].ClusterName != strings.ReplaceAll(sourceKey, "/", ".") {
		err = UnsupportedMultiplePeer(targetKey)
		logger.Errorf("[RunRBDMirrorPeerAddProcess] Error occurred during check peer info. Cause: %+v", err)
		return err
	}

	if p, err = getRBDMirrorPoolInfo(source["pool"].(string), sourceKey, getCephClient(source)); err != nil {
		return err
	}

	if p.Mode != "image" {
		logger.Infof("[RunRBDMirrorPeerAddProcess] peer info: mode(%s)", p.Mode)
		if _, err = execCommand(rbdPoolEnableCommand(source["pool"].(string), sourceKey, getCephClient(source))); err != nil {
			logger.Errorf("[RunRBDMirrorPeerAddProcess] Could not exec command: rbdPoolEnableCommand. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[RunRBDMirrorPeerAddProcess] Success: source(%s) target(%s)", sourceKey, targetKey)
	return nil
}

// RunRBDMirrorPeerRemoveProcess source, target mirror pool peer 삭제 함수
func RunRBDMirrorPeerRemoveProcess(sourceKey, targetKey string, source, target map[string]interface{}) error {
	logger.Infof("[RunRBDMirrorPeerRemoveProcess] Start: source(%s) target(%s)", sourceKey, targetKey)

	var err error
	var tp *peerInfoResult

	if tp, err = getRBDMirrorPoolInfo(target["pool"].(string), targetKey, getCephClient(target)); err != nil {
		return err
	}

	// n:1 에 peer 등록은 불가능 하여 [0] 인덱스 값만 확인
	if len(tp.Peers) > 0 {
		logger.Infof("[RunRBDMirrorPeerRemoveProcess] len(peer): %d", len(tp.Peers))

		if tp.Peers[0].ClientName != "client."+source["client"].(string) || tp.Peers[0].ClusterName != strings.ReplaceAll(sourceKey, "/", ".") {
			err = NotMatchingPeerInfo(tp.Peers[0].ClientName, tp.Peers[0].ClusterName)
			logger.Errorf("[RunRBDMirrorPeerRemoveProcess] Error occurred during check peer info. Cause: %+v", err)
			return err
		}

		if _, err = execCommand(rbdPoolPeerRemoveCommand(target["pool"].(string), targetKey, getCephClient(target), tp.Peers[0].UUID)); err != nil {
			logger.Errorf("[RunRBDMirrorPeerRemoveProcess] Could not exec command: rbdPoolPeerRemoveCommand. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[RunRBDMirrorPeerRemoveProcess] Success: source(%s) target(%s)", sourceKey, targetKey)
	return nil
}

// CheckValidCephMetadata source, target ceph metadata 유효성 검사 함수
func CheckValidCephMetadata(source, target map[string]interface{}) error {
	for _, md := range []map[string]interface{}{source, target} {
		var param string
		if _, ok := md["pool"]; !ok {
			param = "pool"
		}

		if _, ok := md["keyring"]; !ok {
			param = "keyring"
		}

		if _, ok := md["conf"]; !ok {
			param = "conf"
		}

		if _, ok := md["client"]; !ok {
			param = "client"
		}

		if _, ok := md["admin_client"]; ok {
			if _, ok = md["admin_keyring"]; !ok {
				param = "admin_keyring"
			}
		}

		if _, ok := md["admin_keyring"]; ok {
			if _, ok = md["admin_client"]; !ok {
				param = "admin_client"
			}
		}

		if param != "" {
			return errors.RequiredParameter(param)
		}
	}

	if source["pool"] != target["pool"] {
		return DifferentPools(source["pool"].(string), target["pool"].(string))
	}

	return nil
}

func getRBDPoolMirrorImageList(key string, md map[string]interface{}) ([]string, error) {
	logger.Infof("[getRBDPoolMirrorImageList] exec rbdPoolImageListCommand")
	b, err := execCommand(rbdPoolImageListCommand(md["pool"].(string), key, getCephClient(md)))
	if err != nil {
		logger.Errorf("[getRBDPoolMirrorImageList] Could not exec command: rbdPoolImageListCommand. Cause: %+v", err)
		return nil, err
	}

	var imageList []string

	if err = json.Unmarshal(b, &imageList); err != nil {
		logger.Errorf("[getRBDPoolMirrorImageList] Could not unmarshal: imageList. Cause: %+v", err)
		return nil, errors.Unknown(err)
	}

	return imageList, nil
}

func getRBDImageInfo(key, volume string, md map[string]interface{}) (*rbdImageInfo, error) {
	logger.Infof("[getRBDImageInfo] exec rbdImageInfoCommand")
	b, err := execCommand(rbdImageInfoCommand(md["pool"].(string), key, getCephClient(md), volume))
	if err != nil {
		logger.Errorf("[getRBDImageInfo] Could not exec command: rbdImageInfoCommand. Cause: %+v", err)
		return nil, err
	}

	var imageInfo rbdImageInfo
	if err = json.Unmarshal(b, &imageInfo); err != nil {
		logger.Errorf("[getRBDImageInfo] Could not unmarshal: rbdImageInfo. Cause: %+v", err)
		return nil, errors.Unknown(err)
	}

	return &imageInfo, nil

}

// PrepareMirroringRBDImage source, target rbd 이미지 복제 시작 및 설정 함수
func PrepareMirroringRBDImage(sourceKey, targetKey, volume string, source, target map[string]interface{}) error {
	logger.Infof("[PrepareMirroringRBDImage] Run: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)

	var err error

	// 소스 image 정보 반환
	var sourceImageInfo *rbdImageInfo
	if sourceImageInfo, err = getRBDImageInfo(sourceKey, volume, source); err != nil {
		logger.Errorf("[PrepareMirroringRBDImage] Could not get source rbd image info: source(%s). Cause: %+v", sourceKey, err)
		return err
	}

	// 소스 이미지의 mirroring 정보 확인
	// 소스 이미지의 primary 값
	// false: demote
	// true: primary
	// demote 로 설정 된 경우 이미 다른 복제가 진행 중인 것으로 판단
	if sourceImageInfo.Mirroring != nil && sourceImageInfo.Mirroring.State == "enabled" && !sourceImageInfo.Mirroring.Primary {
		err = NotSetPromote(source["pool"].(string), volume)
		logger.Errorf("[PrepareMirroringRBDImage] Error occurred during check source(%s) image info. Cause: %+v", sourceKey, err)
		return err
	}

	// 타겟 pool 에서 이미지 목록 반환
	imageList, err := getRBDPoolMirrorImageList(targetKey, target)
	if err != nil {
		logger.Errorf("[PrepareMirroringRBDImage] Could not get target rbd pool mirror image list: target(%s). Cause: %+v", targetKey, err)
		return err
	}

	// 복제 할 이미지가 target pool 에 존재 하는지 확인
	var match bool
	for _, image := range imageList {
		if volume == image {
			match = true
			break
		}
	}

	// target image 가 존재하고 기존 복제중인 image 인지 확인
	if match {
		var targetImageInfo *rbdImageInfo
		if targetImageInfo, err = getRBDImageInfo(targetKey, volume, target); err != nil {
			logger.Errorf("[PrepareMirroringRBDImage] Could not get target rbd image info: target(%s). Cause: %+v", targetKey, err)
			return err
		}

		if targetImageInfo.Mirroring != nil && targetImageInfo.Mirroring.Primary {
			// target image 가 primary(promote) 상태이면 복제를 위해 demote 상태로 변경시킨다.
			logger.Infof("[PrepareMirroringRBDImage] Change RBD image(%s) to demote state.", targetImageInfo.Name)
			if _, err = execCommand(rbdMirrorImageDemoteCommand(target["pool"].(string), targetKey, getCephClient(target), targetImageInfo.Name)); err != nil {
				logger.Errorf("[PrepareMirroringRBDImage] Could not exec command: rbdMirrorImageDemoteCommand. Cause: %+v", err)
				return err
			}
		}
	}

	logger.Infof("[PrepareMirroringRBDImage] Run next - SetRBDMirrorImageEnable: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)
	return SetRBDMirrorImageEnable(sourceKey, volume, source)
}

func getTargetImageInfoMatchingSourceImage(sourceKey, targetKey, volume string, source, target map[string]interface{}) (*rbdImageInfo, error) {
	// source 이미지 정보 반환
	sourceImageInfo, err := getRBDImageInfo(sourceKey, volume, source)
	if err != nil {
		logger.Errorf("[getTargetImageInfoMatchingSourceImage] Could not get source rbd image info: source(%s). Cause: %+v", sourceKey, err)
		return nil, err
	}

	if sourceImageInfo.Mirroring == nil || sourceImageInfo.Mirroring.State == "disabled" {
		err = NotSetMirrorEnable(source["pool"].(string), volume)
		logger.Errorf("[getTargetImageInfoMatchingSourceImage] Error occurred during check source(%s) image info. Cause: %+v", sourceKey, err)
		return nil, err
	}

	var imageList []string
	imageList, err = getRBDPoolMirrorImageList(targetKey, target)
	if err != nil {
		logger.Errorf("[getTargetImageInfoMatchingSourceImage] Could not get target rbd pool mirror image list: target(%s). Cause: %+v", targetKey, err)
		return nil, err
	}

	// paused 이후 target volume 이름이 변경 됨
	// target pool 에서 전체 이미지를 가져옴
	// target 전체 이미지 중 mirroring global id source 이미지의 mirroring global id가 같은 target image 정보 반환
	var nameEqualImage *rbdImageInfo
	for _, image := range imageList {
		var targetImage *rbdImageInfo

		// image list 를 가져오는 시점과 실제 image 를 조회하는 시점에 target image list 가 변경될 수 있음
		if targetImage, err = getRBDImageInfo(targetKey, image, target); err != nil {
			logger.Warnf("[getTargetImageInfoMatchingSourceImage] Could not get target rbd image info: target(%s). Cause: %+v", targetKey, err)
			continue
		}

		if targetImage.Mirroring != nil && targetImage.Mirroring.GlobalID == sourceImageInfo.Mirroring.GlobalID && targetImage.Mirroring.GlobalID != "" {
			return targetImage, nil
		}

		if image == sourceImageInfo.Name {
			nameEqualImage = targetImage
		}
	}

	switch {
	case nameEqualImage != nil && (nameEqualImage.Mirroring == nil || nameEqualImage.Mirroring.State == "disabled"):
		err = NotSetMirrorEnable(target["pool"].(string), nameEqualImage.Name)
		logger.Errorf("[getTargetImageInfoMatchingSourceImage] Error occurred during check target(%s) image info. Cause: %+v", targetKey, err)
		return nil, err

	// target 에 source image name 이 있지만, state 가 "disabling" 상태 일 경우
	// 이전에 제거되지 못한 mirroring 볼륨이 Read-Only 상태로 남아있는 케이스로
	// source 측 mirror image 를 disable 한다.
	case nameEqualImage != nil && nameEqualImage.Mirroring.State == "disabling":
		logger.Infof("[getTargetImageInfoMatchingSourceImage] exec rbdMirrorImageDisableCommand")
		if _, err = execCommand(rbdMirrorImageDisableCommand(source["pool"].(string), sourceKey, getCephClient(source), volume)); err != nil {
			logger.Errorf("[getTargetImageInfoMatchingSourceImage] Could not exec command: rbdMirrorImageDisableCommand. Cause: %+v", err)
			return nil, err
		}
		err = NotSetMirrorEnable(target["pool"].(string), nameEqualImage.Name)
		logger.Errorf("[getTargetImageInfoMatchingSourceImage] Error occurred during check target(%s) image info. Cause: %+v", targetKey, err)
		return nil, err

	case nameEqualImage != nil:
		return nameEqualImage, nil
	}

	err = NotFoundMirrorImage(target["pool"].(string), volume)
	logger.Errorf("[getTargetImageInfoMatchingSourceImage] Could not get target(%s) image info matching source(%s) image. Cause: %+v", targetKey, sourceKey, err)
	return nil, err
}

// SetTargetRBDImageDemote source global id 와 일치 혹은 source image와 일치 하는 target image 를 demote 로 설정 하는 함수
func SetTargetRBDImageDemote(sourceKey, targetKey, volumeUUID string, source, target map[string]interface{}) error {
	logger.Infof("[SetTargetRBDImageDemote] Start: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volumeUUID)
	targetInfo, err := getTargetImageInfoMatchingSourceImage(sourceKey, targetKey, volumeUUID, source, target)
	if err != nil {
		return err
	}

	if targetInfo.Mirroring.Primary {
		if _, err = execCommand(rbdMirrorImageDemoteCommand(target["pool"].(string), targetKey, getCephClient(target), targetInfo.Name)); err != nil {
			logger.Errorf("[SetTargetRBDImageDemote] Could not exec command: rbdMirrorImageDemoteCommand. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[SetTargetRBDImageDemote] Success: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volumeUUID)
	return nil
}

// GetRBDMirrorImageStatus rbd image 상태 조회 함수
func GetRBDMirrorImageStatus(sourceKey, targetKey, volumeUUID string, source, target map[string]interface{}) (string, string, error) {
	logger.Infof("[GetRBDMirrorImageStatus] Start: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volumeUUID)

	targetInfo, err := getTargetImageInfoMatchingSourceImage(sourceKey, targetKey, volumeUUID, source, target)
	if err != nil {
		return "", "", err
	}

	b, err := execCommand(rbdMirrorImageStatusCommand(target["pool"].(string), targetKey, getCephClient(target), targetInfo.Name))
	if err != nil {
		logger.Errorf("[GetRBDMirrorImageStatus] Could not exec command: rbdMirrorImageStatusCommand. Cause: %+v", err)
		return "", "", err
	}

	var status rbdMirrorStatusInfo
	if err = json.Unmarshal(b, &status); err != nil {
		logger.Errorf("[GetRBDMirrorImageStatus] Could not unmarshal: rbdMirrorStatusInfo. Cause: %+v", err)
		return "", "", errors.Unknown(err)
	}

	logger.Infof("[GetRBDMirrorImageStatus] Success: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volumeUUID)
	return status.State, status.Description, nil
}

// ReSyncTargetRBDImage source, target split brain 처리 함수
func ReSyncTargetRBDImage(sourceKey, targetKey, volumeUUID string, source, target map[string]interface{}) error {
	logger.Infof("[ReSyncTargetRBDImage] Start: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volumeUUID)

	targetInfo, err := getTargetImageInfoMatchingSourceImage(sourceKey, targetKey, volumeUUID, source, target)
	if err != nil {
		return err
	}

	if _, err = execCommand(rbdMirrorImageReSyncCommand(target["pool"].(string), targetKey, getCephClient(target), targetInfo.Name)); err != nil {
		logger.Errorf("[ReSyncTargetRBDImage] Could not exec command: rbdMirrorImageReSyncCommand. Cause: %+v", err)
		return err
	}

	logger.Infof("[ReSyncTargetRBDImage] Success: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volumeUUID)
	return nil
}

func getRBDImageSnapshotList(key, volume string, md map[string]interface{}) ([]rbdImageSnapshotInfo, error) {
	logger.Infof("[getRBDImageSnapshotList] exec rbdMirrorImageSnapshotListCommand")

	b, err := execCommand(rbdMirrorImageSnapshotListCommand(md["pool"].(string), key, getCephClient(md), volume))
	if err != nil {
		logger.Errorf("[getRBDImageSnapshotList] Could not exec command: rbdMirrorImageSnapshotListCommand. Cause: %+v", err)
		return nil, err
	}

	var list []rbdImageSnapshotInfo
	if err = json.Unmarshal(b, &list); err != nil {
		logger.Errorf("[getRBDImageSnapshotList] Could not unmarshal: rbdImageSnapshotInfo. Cause: %+v", err)
		return nil, errors.Unknown(err)
	}

	return list, nil
}

// DeleteRBDImage rbd image 를 삭제 하는 함수
// rbd image 의 스냅샷 목록을 가져 온 후
// 스냅샷이 protect 로 설정 되어 있을 경우 해당 스냅샷을 unprotect 로 설정
// rbd image 삭제
func DeleteRBDImage(key, volume string, md map[string]interface{}) error {
	logger.Infof("[DeleteRBDImage] Start: key(%s) volume(%s)", key, volume)

	imageInfo, err := getRBDImageInfo(key, volume, md)
	if err != nil {
		return internal.UnableGetVolumeImage(volume)
	}

	// mirroring 을 pause 한 뒤 삭제하는 경우 demote(read-only) 상태의 image 를 promote 로 변경한 뒤
	// image 를 직접 disable 해야 함
	if imageInfo.Mirroring != nil && imageInfo.Mirroring.State == "enabled" {
		if !imageInfo.Mirroring.Primary {
			if _, err = execCommand(rbdMirrorImagePromoteCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
				logger.Errorf("[DeleteRBDImage] Could not exec command: rbdMirrorImagePromoteCommand. Cause: %+v", err)
				return err
			}
		}

		if _, err = execCommand(rbdMirrorImageDisableCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
			logger.Errorf("[DeleteRBDImage] Could not exec command: rbdMirrorImageDisableCommand. Cause: %+v", err)
			return err
		}
	}

	for _, f := range imageInfo.Features {
		if f == "journaling" {
			if _, err = execCommand(rbdMirrorImageFeatureDisableJournaling(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
				logger.Errorf("[DeleteRBDImage] Could not exec command: rbdMirrorImageFeatureDisableJournaling. Cause: %+v", err)
				return err
			}
			break
		}
	}

	var snapshotList []rbdImageSnapshotInfo
	snapshotList, err = getRBDImageSnapshotList(key, volume, md)
	if err != nil {
		return err
	}

	for _, s := range snapshotList {
		if s.Protected == "true" {
			if _, err = execCommand(rbdMirrorImageUnprotectSnapshotCommand(md["pool"].(string), key, getCephClient(md), volume, s.Name)); err != nil {
				logger.Errorf("[DeleteRBDImage] Could not exec command: rbdMirrorImageUnprotectSnapshotCommand. Cause: %+v", err)
				return err
			}
		}
	}

	if _, err = execCommand(rbdMirrorImageObjectMapRebuild(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
		logger.Errorf("[DeleteRBDImage] Could not exec command: rbdMirrorImageObjectMapRebuild. Cause: %+v", err)
		return err
	}

	if _, err = execCommand(rbdMirrorImageSnapshotPurgeCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
		logger.Errorf("[DeleteRBDImage] Could not exec command: rbdMirrorImageSnapshotPurgeCommand. Cause: %+v", err)
		return err
	}

	if _, err = execCommand(rbdMirrorImageDeleteCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
		logger.Errorf("[DeleteRBDImage] Could not exec command: rbdMirrorImageDeleteCommand. Cause: %+v", err)
		return err
	}

	logger.Infof("[DeleteRBDImage] Success: key(%s) volume(%s)", key, volume)
	return nil
}

// SetRBDMirrorImageEnable rbd mirror image 가 enable 로 설정 안되어 있을 경우 enable 로 설정 하는 함수
func SetRBDMirrorImageEnable(key, volume string, md map[string]interface{}) error {
	logger.Infof("[SetRBDMirrorImageEnable] Start: key(%s) volume(%s)", key, volume)

	var err error

	var sourceImageInfo *rbdImageInfo
	if sourceImageInfo, err = getRBDImageInfo(key, volume, md); err != nil {
		return err
	}

	var match bool
	if sourceImageInfo.Mirroring == nil || (sourceImageInfo.Mirroring != nil && sourceImageInfo.Mirroring.State != "enabled") {
		for _, f := range sourceImageInfo.Features {
			if f == "journaling" {
				match = true
				break
			}
		}

		// 이미지의 feature 로 journaling 이 안되어 있으면 feature 설정
		if !match {
			c := rbdMirrorImageFeatureEnableJournaling(md["pool"].(string), key, getCephClient(md), volume)
			if _, err = execCommand(c); err != nil {
				logger.Errorf("[SetRBDMirrorImageEnable] Could not exec command: rbdMirrorImageFeatureEnableJournaling. Cause: %+v", err)
				return err
			}
		}

		// 소스 이미지 mirror image enable 설정
		if _, err = execCommand(rbdMirrorImageEnableCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
			logger.Errorf("[SetRBDMirrorImageEnable] Could not exec command: rbdMirrorImageEnableCommand. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[SetRBDMirrorImageEnable] Success: key(%s) volume(%s)", key, volume)
	return nil
}

// SetRBDMirrorImagePromote rbd image 를 promote 로 바꾸는 함수
// mirroring 자체가 설정이 안되어 있거나, mirroring 이 disable 상태 면 에러를 반환 하며,
// mirroring 이 설정 되어 있고, demote 일 경우만 promote 로 변경 한다.
func SetRBDMirrorImagePromote(key, volume string, md map[string]interface{}) error {
	logger.Infof("[SetRBDMirrorImagePromote] Start: key(%s) volume(%s)", key, volume)

	var err error
	var imageInfo *rbdImageInfo
	if imageInfo, err = getRBDImageInfo(key, volume, md); err != nil {
		return err
	}

	if imageInfo.Mirroring == nil || imageInfo.Mirroring.State == "disabled" {
		err = NotSetMirrorEnable(md["pool"].(string), volume)
		logger.Errorf("[SetRBDMirrorImagePromote] Error occurred during check rbd image info: key(%s) volume(%s). Cause: %+v", key, volume, err)
		return err
	}

	if imageInfo.Mirroring != nil && !imageInfo.Mirroring.Primary {
		if _, err = execCommand(rbdMirrorImagePromoteCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
			logger.Errorf("[SetRBDMirrorImagePromote] Could not exec command: rbdMirrorImagePromoteCommand. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[SetRBDMirrorImagePromote] Success: key(%s) volume(%s)", key, volume)
	return nil
}

// RunRBDMirrorImageDisable 는 rbd image 의 mirror enable 를 disable 로 설정 하는 함수
func RunRBDMirrorImageDisable(key, volume string, md map[string]interface{}) error {
	logger.Infof("[RunRBDMirrorImageDisable] Start: key(%s) volume(%s)", key, volume)

	var err error
	var imageInfo *rbdImageInfo
	if imageInfo, err = getRBDImageInfo(key, volume, md); err != nil {
		return err
	}

	if imageInfo.Mirroring != nil && imageInfo.Mirroring.State == "enabled" {
		if !imageInfo.Mirroring.Primary {
			if _, err = execCommand(rbdMirrorImagePromoteCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
				logger.Errorf("[RunRBDMirrorImageDisable] Could not exec command: rbdMirrorImagePromoteCommand. Cause: %+v", err)
				return err
			}
		}

		if _, err = execCommand(rbdMirrorImageDisableCommand(md["pool"].(string), key, getCephClient(md), volume)); err != nil {
			logger.Errorf("[RunRBDMirrorImageDisable] Could not exec command: rbdMirrorImageDisableCommand. Cause: %+v", err)
			return err
		}
	}

	logger.Infof("[RunRBDMirrorImageDisable] Success: key(%s) volume(%s)", key, volume)
	return nil
}

// StopRBDMirrorImage source, target image 복제를 중지 하는 함수
// target image 는 삭제 하지 않고 남겨 놓는다.
func StopRBDMirrorImage(sourceKey, targetKey, volume string, source, target map[string]interface{}, vol *mirror.Volume) error {
	logger.Infof("[StopRBDMirrorImage] Start: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)

	targetInfo, err := getTargetImageInfoMatchingSourceImage(sourceKey, targetKey, volume, source, target)
	switch {
	case err != nil && (errors.Equal(err, ErrNotFoundMirrorImage) || errors.Equal(err, ErrNotSetMirrorEnable) || errors.Equal(err, ErrTimeoutExecCommand)):
		break

	case err != nil:
		return err

	default:
		var targetVolume = volume
		if targetInfo.Name != volume {
			targetVolume = targetInfo.Name
		}

		logger.Infof("[StopRBDMirrorImage] Run >> RunRBDMirrorImageDisable")
		if err = RunRBDMirrorImageDisable(targetKey, targetVolume, target); err != nil {
			return err
		}
	}

	// source image 가 다른 mirroring 이 설정되지 않은 경우 source image mirror disable 설정한다.
	_, err = vol.GetRefCount()
	switch {
	case errors.Equal(err, mirror.ErrNotFoundSourceVolumeReference):
		logger.Infof("[StopRBDMirrorImage] Not found source volume reference >> Run: RunRBDMirrorImageDisable: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)
		if err = RunRBDMirrorImageDisable(sourceKey, volume, source); err != nil {
			return err
		}
		break

	case err != nil:
		logger.Errorf("[StopRBDMirrorImage] Could not get reference count: mirror_volume/storage.%d/volume.%d. Cause: %+v",
			vol.SourceClusterStorage.StorageID, vol.SourceVolume.VolumeID, err)
		return err
	}

	logger.Infof("[StopRBDMirrorImage] Success: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)
	return nil
}

// StopAndDeleteRBDImage source, target image 복제를 중지 및 삭제 하는 함수
func StopAndDeleteRBDImage(sourceKey, targetKey, volume string, source, target map[string]interface{}, vol *mirror.Volume) error {
	logger.Infof("[StopAndDeleteRBDImage] Start: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)

	targetInfo, err := getTargetImageInfoMatchingSourceImage(sourceKey, targetKey, volume, source, target)
	switch {
	case err != nil && (errors.Equal(err, ErrNotFoundMirrorImage) || errors.Equal(err, ErrNotSetMirrorEnable) || errors.Equal(err, ErrTimeoutExecCommand)):
		break

	case err != nil:
		return err
	}

	// source image 가 다른 mirroring 이 설정되지 않은 경우 source image mirror disable 설정한다.
	_, err = vol.GetRefCount()
	switch {
	case errors.Equal(err, mirror.ErrNotFoundSourceVolumeReference):
		logger.Infof("[StopAndDeleteRBDImage] Not found source volume reference >> Run: RunRBDMirrorImageDisable: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)
		if err = RunRBDMirrorImageDisable(sourceKey, volume, source); err != nil {
			return err
		}

		// mirroring 이 disable 되고 target image 가 지워질 때까지 시간 차이가 발생하기 때문에
		// target image 가 지워질 때 까지 임시적으로 sleep 한다.
		time.Sleep(time.Second)

		break

	case err != nil:
		logger.Errorf("[StopAndDeleteRBDImage] Could not get reference count: mirror_volume/storage.%d/volume.%d. Cause: %+v",
			vol.SourceClusterStorage.StorageID, vol.SourceVolume.VolumeID, err)
		return err
	}

	// target image name 정보가 있는 경우 해당 정보를 사용하고,
	// 없는 경우 source image name 을 사용
	var targetVolume string
	if targetInfo != nil {
		targetVolume = targetInfo.Name
	} else {
		targetVolume = volume
	}

	// mirroring 이 정상적인 상태에서 source image mirror 를 disable 하는 경우 target image 가 자동으로 삭제되지만
	// source image 가 다른 mirroring 이 설정되어 있거나,
	// source mirroring 해제 후에도 pause, split brain 등 이유로 target image 가 남아있는 경우
	// target image 를 직접 삭제한다.
	logger.Infof("[StopAndDeleteRBDImage] Run >> DeleteRBDImage: target(%s)", targetKey)
	err = DeleteRBDImage(targetKey, targetVolume, target)
	switch {
	case err != nil && errors.Equal(err, internal.ErrUnableGetVolumeImage):
		break

	case err != nil:
		logger.Warnf("[StopAndDeleteRBDImage] Could not delete RBD image(%s). cause: %v", targetVolume, err)
	}

	logger.Infof("[StopAndDeleteRBDImage] Success: source(%s) target(%s) volume(%s)", sourceKey, targetKey, volume)
	return nil
}
