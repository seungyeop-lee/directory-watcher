# 언어

- 프로젝트의 기본 언어는 한국어다. 코드 주석, 커밋 메시지, 내부 문서는 한국어로 쓴다.
- 외부에 보이거나 배포되는 부분만 영어로 쓴다. CLI 출력·로그·오류 메시지와 릴리스 노트가 여기에 해당한다.
- 한국어판(`.ko.md`)과 영어판(`.md`)이 함께 있는 문서는 한쪽을 고치면 다른 쪽도 같이 고쳐, 두 판의 절 구성과 사실을 같게 유지한다.

# 검증

- Windows 동작을 확인해야 하면, Windows 환경이 없다고 판단하기 전에 `/Applications/UTM.app/Contents/MacOS/utmctl list`로 UTM VM을 찾는다. VM이 있으면 [UTM Windows VM에서 E2E 실행](e2e/docs/windows-utm.ko.md)을 읽고 `make e2e-windows-all ARGS="-vm=<VM 이름>"`으로 게스트에서 테스트를 실행한다. `./e2e/scenarios` 밖의 패키지를 함께 실행하는 방법과 이 VM으로 대신할 수 없는 Windows 검증도 그 문서에 있다.
- 심볼릭 링크를 만드는 테스트는 링크를 만들 수 없으면 SKIP한다. Windows 실행 출력에서 해당 테스트가 `--- SKIP` 없이 `--- PASS`인지 확인한다.
