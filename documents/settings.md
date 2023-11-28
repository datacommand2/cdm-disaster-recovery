# 복구 설정
## 일반
#### 모니터링
| Property | Description | Value | Default | Not null | CUD Role |
|---|---|---|---|---|---|
| `dr-global-monitoring-interval` | 실시간 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | 5 | `True` | `Manager`, `Operator` |
| `dr-solution-monitoring-interval` | 솔루션 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | null | `False` | `Manager`, `Operator` |
| `dr-cluster-monitoring-interval` | 클러스터 복제현황 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | null | `False` | `Manager`, `Operator` |
| `dr-storage-monitoring-interval` | 스토리지 복제현황 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | null | `False` | `Manager`, `Operator` |
| `dr-availability-zone-monitoring-interval` | 가용구역 복제현황 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | null | `False` | `Manager`, `Operator` |
| `dr-tenant-monitoring-interval` | 테넌트 복제현황 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | null | `False` | `Manager`, `Operator` |
| `dr-instance-monitoring-interval` | 인스턴스 복제현황 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | null | `False` | `Manager`, `Operator` |
| `dr-volume-monitoring-interval` | 볼륨 복제현황 모니터링 주기간격 기본값 | 주기간격(2-30(s)) | null | `False` | `Manager`, `Operator` |


## 보고서
| Property | Description | Value | Default | Not null | CUD Role |
|---|---|---|---|---|---|
| `dr-replication-daily-report-schedule-enable` | 데이터 복제결과 일간 보고서 생성 스케쥴 활성화 여부 | 활성화 여부(True, False) | False | `True` | `Manager`, `Operator` |
| `dr-replication-daily-report-schedule` | 데이터 복제결과 일간 보고서 생성 스케쥴 | { "start-year": "시작년도(YYYY)", "start-month": "시작월(MM)", "start-day": "시작일(DD)", "hour": "시(0-23(hour))", "minute": "분(0-59(min))" } | {} | `True` | `Manager`, `Operator` |
| `dr-replication-weekly-report-schedule-enable` | 데이터 복제결과 주간 보고서 생성 스케쥴 활성화 여부 | 활성화 여부(True, False) | False | `True` | `Manager`, `Operator` ||
| `dr-replication-weekly-report-schedule` | 데이터 복제결과 주간 보고서 생성 스케쥴 | { "start-month": "시작월(MM)", "start-day": "시작일(DD)", "day-of-week": "요일(Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday)", "hour": "시(0-23(hour))", "minute": "분(0-59(min))" } | {} | `True` | `Manager`, `Operator` |
| `dr-replication-monthly-report-schedule-enable` | 데이터 복제결과 월간 보고서 생성 스케쥴 활성화 여부 | 활성화 여부(True, False) | False | `True` | `Manager`, `Operator` |
| `dr-replication-monthly-report-schedule` | 데이터 복제결과 월간 보고서 생성 스케쥴 | { "type": "스케쥴 종류(Day of Monthly, Week of Monthly)", "start-year": "시작년도(YYYY)", "start-month": "시작월(MM)", "week-of-month": "[Week of monthly] 주차(First Week, Second Week, Third Week, Fourth Week, Fifth Week, Last Week)", "day-of-month": "[Day of Monthly] 일자(1-31(day), Last day)", "day-of-week": "[Week of Monthly] 요일(Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday)", "hour": "시(0-23(hour))", "minute": "분(0-59(min))" } | {} | `True` | `Manager`, `Operator` |
| `dr-replication-quarterly-report-schedule-enable` | 데이터 복제결과 분기 보고서 생성 스케쥴 활성화 여부 | 활성화 여부(True, False) | False | `True` | `Manager`, `Operator` |
| `dr-replication-quarterly-report-schedule` | 데이터 복제결과 분기 보고서 생성 스케쥴 | { "type": "스케쥴 종류(Day of Monthly, Week of Monthly)", "start-year": "시작년도(YYYY)", "start-month": "시작월(MM)", "week-of-month": "[Week of monthly] 주차(First Week, Second Week, Third Week, Fourth Week, Fifth Week, Last Week)", "day-of-month": "[Day of Monthly] 일자(1-31(day), Last day)", "day-of-week": "[Week of Monthly] 요일(Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday)", "hour": "시(0-23(hour))", "minute": "분(0-59(min))" } | {} | `True` | `Manager`, `Operator` |
| `dr-replication-semiannual-report-schedule-enable` | 데이터 복제결과 반기 보고서 생성 스케쥴 활성화 여부 | 활성화 여부(True, False) | False | `True` | `Manager`, `Operator` |
| `dr-replication-semiannual-report-schedule` | 데이터 복제결과 반기 보고서 생성 스케쥴 | { "type": "스케쥴 종류(Day of Monthly, Week of Monthly)", "start-year": "시작년도(YYYY)", "start-month": "시작월(MM)", "week-of-month": "[Week of monthly] 주차(First Week, Second Week, Third Week, Fourth Week, Fifth Week, Last Week)", "day-of-month": "[Day of Monthly] 일자(1-31(day), Last day)", "day-of-week": "[Week of Monthly] 요일(Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday)", "hour": "시(0-23(hour))", "minute": "분(0-59(min))" } | {} | `True` | `Manager`, `Operator` |
| `dr-replication-annual-report-schedule-enable` | 데이터 복제결과 연간 보고서 생성 스케쥴 활성화 여부 | 활성화 여부(True, False) | False | `True` | `Manager`, `Operator` |
| `dr-replication-annual-report-schedule` | 데이터 복제결과 연간 보고서 생성 스케쥴 | { "type": "스케쥴 종류(Day of Monthly, Week of Monthly)", "start-year": "시작년도(YYYY)", "start-month": "시작월(MM)", "week-of-month": "[Week of monthly] 주차(First Week, Second Week, Third Week, Fourth Week, Fifth Week, Last Week)", "day-of-month": "[Day of Monthly] 일자(1-31(day), Last day)", "day-of-week": "[Week of Monthly] 요일(Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday)", "hour": "시(0-23(hour))", "minute": "분(0-59(min))" } | {} | `True` | `Manager`, `Operator` |
| `dr-replication-report-store-period` | 데이터 복제결과 보고서 보유기간 | 보유기간(1-10(year), Unlimited) | Unlimited | `True` | `Manager`, `Operator` |
| `dr-recovery-traing-report-store-period` | 모의훈련결과 보고서 보유기간 | 보유기간(1-10(year), Unlimited) | Unlimited | `True` | `Manager`, `Operator` |


## 정책
| Property | Description | Value | Default | Not null | CUD Role |
|---|---|---|---|---|---|
| `dr-deleted-target-instance-pool-detected-polish` | 복구대상 클러스터 인스턴스 풀의 제거 감지시 정책. 제거된 인스턴스 풀과 매핑된 인스턴스들을 동일한 가용구역내 인스턴스 풀들로 재분배하거나 해제한다. *(충족하는 인스턴스 풀이 없는 인스턴스들은 해제된다)* | 정책(Redistribute, Unmapping) | Redistribute | `True` | `Manager`, `Operator` |
| `dr-changed-target-instance-pool-resource-detected-polish` | 복구대상 클러스터 인스턴스 풀의 리소스 변경시 정책. 인스턴스들이 필요로하는 리소스를 충족하지 못하는 경우 일부 인스턴스들을 동일한 가용구역내 다른 인스턴스 풀에 재분배하거나 해제한다. *(충족하는 인스턴스 풀이 없다면 해제된다)* | 정책(Redistribute, Unmapping) | Redistribute | `True` | `Manager`, `Operator` |
| `dr-created-source-instance-detected-polish` | 원본 클러스터에 복구대상으로 설정할 수 있는 인스턴스 추가 감지시 정책. 추가된 인스턴스의 가용구역과 매핑된 복구대상 클러스터의 가용구역내 인스턴스 풀에 자동매핑하거나 아무런 동작도 하지 않는다. *(충족하는 인스턴스 풀이 없다면 매핑하지 않는다)* | 정책(Auto mapping, No action) | Auto mapping | `True` | `Manager`, `Operator` |
| `dr-instance-auto-mapping-filter` | 신규 인스턴스 자동매핑 대상 선정방식 | { "availability-zone": [ "자동매핑할 가용구역 (없으면 전체)" ], "exclude-availability-zone": [ "자동매핑하지 않을 가용구역" ], "tenant": [ "자동매핑할 테넌트 (없으면 전체)" ], "exclude-tenant": [ "자동매핑하지 않을 테넌트" ], "instance-name-regex": [ "자동매핑할 인스턴스 이름 정규표현식 (없으면 전체)" ], "exclude-instance-name-regex": [ "자동매핑하지 않을 인스턴스 이름 정규표현식" ] } | { "availability-zone": [], "exclude-availability-zone": [], "tenant": [], "exclude-tenant": [], "instance-name-regex": [], "exclude-instance-name-regex": [] } | `True` | `Manager`, `Operator` |
| `dr-changed-source-instance-resource-detected-polish` | 복구대상으로 설정된 원본 클러스터 인스턴스의 필요 리소스 변경 감지시 정책. 매핑된 인스턴스 풀이 변경된 필요 리소스를 충족하지 못하는 경우 인스턴스를 동일한 가용구역내 다른 인스턴스 풀로 자동변경하거나 해체한다. *(충족하는 인스턴스 풀이 없다면 해제된다)* | 정책(Auto remapping, Unmapping) | Auto remapping | `True` | `Manager`, `Operator` |
