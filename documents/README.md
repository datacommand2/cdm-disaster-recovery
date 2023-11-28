# CDM-DisasterRecovery
(제품 설명과 제품의 특장점, 도입시 이점 등을 작성해야 함)

---

## 시스템 요구사항
CDM-DisasterRecovery 는 고가용성을 위해 [CDM-Cloud](@cdm/cdm-cloud)에 2 COPY 이상 배포할 것을 권장하며, 재해복구를 위해 재해복구센터에도 별도의 CDM-Cloud 를 구성하여 배포할 것을 권장한다.

CDM-DisasterRecovery 의 원활한 구동을 위해 필요한 서버의 사양은 다음과 같다.
  - CPU: 최소 4 core / 권장 8 core
  - Memory: 최소 8 Gib / 권장 16 Gib
  - 네트워크: 최소 1G / 권장 10G
  - 스토리지: 최소 100Gib / 권장 1Tib

네트워크의 안정성을 위해 2개 이상의 NIC 를 하나의 interface 에 묶어서(bonding) 사용할 것을 권장하며, 데이터 복제를 위한 네트워크 망을 별도로 운영할 것을 권장한다.

지원하는 클러스터 플랫폼은 다음과 같다.
- OpenStack
- ~~OpenShift~~
- ~~Kubernetes~~
- ~~VMware~~

지원하는 OpenStack 의 버전은 다음과 같다.
- Red Hat OpenStack Platform 13 (Queens) ~2021. 6. 27
- *~~Red Hat OpenStack Platform 14 (Rocky) ~2020. 1. 10~~*
- *Red Hat OpenStack Platform 15 (Stein) ~2020. 11. 19*
- OpenStack 12.x.x (Queens) ~2018. 2. 28
- OpenStack 13.x.x (Rocky) ~2020. 2. 24
- OpenStack 14.x.x (Stein) ~2020. 10. 10
- OpenStack 15.x.x (Train) ~2021. 4. 16
- OpenStack 16.x.x (Ussuri)
> *Red Hat OpenStack Platform 15 (Stein)* 은 금년말 지원이 종료되는 버전이므로 우선순위가 낮다.


## 주요 문서
- [**기능목록**](functions.md)  
  구현해야 하는 기능에 대해 상세하게 설명하는 문서
- [**API**](api/openapi.yaml)  
  CDM-DisasterRecovery 의 API 문서
- [**DB**](database/database.md)  
  CDM-DisasterRecovery 의 DB 문서