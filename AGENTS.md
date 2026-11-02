# 프로젝트별 실행 허용

- 사용자는 worktree에서 만든 작업 브랜치의 일반 push와 승인된 작업 범위 안의 실험을 추가 확인 없이 진행하도록 허용했다. 일반 push 허용은 beads-starter의 일반적인 `git push` 금지에 대한 이 프로젝트의 예외다.
- 이 허용은 작업 범위나 이슈 승인을 확대하지 않는다. 이슈 승인·검수 절차와 Git hook 보호 규칙은 그대로 유지한다.
- 주 통합 브랜치에 대한 직접 push, force push, hook 우회는 허용하지 않는다.
- worker는 push하지 않는다. 브랜치 게시와 CI 실행은 orchestrator가 담당한다.
