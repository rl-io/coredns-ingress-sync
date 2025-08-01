# Changelog

## [0.1.10](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.9...coredns-ingress-sync-v0.1.10) (2025-08-01)


### Bug Fixes

* **helm:** add proper Helm hooks for autoConfigure mode ([a4f14bf](https://github.com/rl-io/coredns-ingress-sync/commit/a4f14bfa5d98f865bfd17309a838bbd8dbe84700))
* **helm:** add proper Helm hooks for autoConfigure mode ([659ff25](https://github.com/rl-io/coredns-ingress-sync/commit/659ff256fc8e3f10e346e3f9319fb32616b09a6f))
* **release:** update release configuration for better versioning and changelog management ([#46](https://github.com/rl-io/coredns-ingress-sync/issues/46)) ([bd95864](https://github.com/rl-io/coredns-ingress-sync/commit/bd95864c26516be83db5d2d931a576f4f6881aaf))

## [0.1.9](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.8...coredns-ingress-sync-v0.1.9) (2025-08-01)


### Bug Fixes

* **ci:** add docker buildx setup to resolve cache export error ([#42](https://github.com/rl-io/coredns-ingress-sync/issues/42)) ([946015f](https://github.com/rl-io/coredns-ingress-sync/commit/946015fd63d5bf18a7b0a7591d35361dc289e6bc))

## [0.1.8](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.7...coredns-ingress-sync-v0.1.8) (2025-08-01)


### Miscellaneous

* **workflows:** update event types for build and release workflows ([dbf8c5e](https://github.com/rl-io/coredns-ingress-sync/commit/dbf8c5e174fb457653037c9b1314cb9bed610377))
* **workflows:** update event types for build and release workflows ([ad27886](https://github.com/rl-io/coredns-ingress-sync/commit/ad278865d885d5c6406d38e8b7551f4d21231bba))

## [0.1.7](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.6...coredns-ingress-sync-v0.1.7) (2025-08-01)


### Features

* **ci:** add repository dispatch event for build and push in workflows ([ce0365c](https://github.com/rl-io/coredns-ingress-sync/commit/ce0365c09e5d3f6f9176771236f45a36bfd27838))
* **ci:** add repository dispatch event for build and push in workflows ([86e7b5c](https://github.com/rl-io/coredns-ingress-sync/commit/86e7b5c70dd34492d754888f644cbe0e5a3ac6b2))


### Miscellaneous

* **config:** update release configuration to include changelog sections ([544f9f8](https://github.com/rl-io/coredns-ingress-sync/commit/544f9f844ad4f9aa2afd265f157f4d956cefa24e))
* **config:** update release configuration to include changelog sections ([a8354a4](https://github.com/rl-io/coredns-ingress-sync/commit/a8354a470dcb68d3ac591744b5396656b906ad7b))

## [0.1.6](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.5...coredns-ingress-sync-v0.1.6) (2025-08-01)


### Bug Fixes

* **ci:** update tag prefix for build and CI/CD workflows to 'coredns-ingress-sync-v*' ([c253f91](https://github.com/rl-io/coredns-ingress-sync/commit/c253f9130676647cfc9e4b8f40fd0412d2e4151d))
* **ci:** update tag prefix for build and CI/CD workflows to 'coredns-ingress-sync-v*' ([7759083](https://github.com/rl-io/coredns-ingress-sync/commit/7759083c9dc15a238a3e6b0c76bd980d4bba4839))

## [0.1.5](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.4...coredns-ingress-sync-v0.1.5) (2025-08-01)


### Bug Fixes

* **ci:** update release conditions to use specific tag prefix for coredns-ingress-sync ([fc0c530](https://github.com/rl-io/coredns-ingress-sync/commit/fc0c5303b485609334afdc8400bc70a00ec79dc7))
* **ci:** update release conditions to use specific tag prefix for coredns-ingress-sync ([c991440](https://github.com/rl-io/coredns-ingress-sync/commit/c991440b801b5d68fefcaa8b2b1f54607b1353be))

## [0.1.4](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.3...coredns-ingress-sync-v0.1.4) (2025-08-01)


### Bug Fixes

* **ci:** update version output format and enhance condition for triggering CI in release-please workflow ([7de28f6](https://github.com/rl-io/coredns-ingress-sync/commit/7de28f6b8853ff26e657b331a3b8b47edc77f9c2))
* **ci:** update version output format and enhance condition for triggering CI in release-please workflow ([4eb20cb](https://github.com/rl-io/coredns-ingress-sync/commit/4eb20cbaed553dc810da09e6f5595407bedf3050))

## [0.1.3](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.2...coredns-ingress-sync-v0.1.3) (2025-07-31)


### Bug Fixes

* **ci:** correct JSON parsing for PR number and branch reference in release-please workflow ([7cddd74](https://github.com/rl-io/coredns-ingress-sync/commit/7cddd74833eae361b7b2a74ddbcd640f1a0c8f86))

## [0.1.2](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.1...coredns-ingress-sync-v0.1.2) (2025-07-31)


### Features

* add new GitHub Actions for Docker image build, security scanning, and testing workflows ([d89bd48](https://github.com/rl-io/coredns-ingress-sync/commit/d89bd480271b12c5329ee8de645ced70bee00d0e))
* add reusable action to update PR status for CI/CD pipeline and split out into modular files ([aa3e534](https://github.com/rl-io/coredns-ingress-sync/commit/aa3e5346f9f99876cb60e7fc8af048cfa9788fdf))
* add reusable action to update PR status for CI/CD pipeline and split out into modular files ([0d80ba1](https://github.com/rl-io/coredns-ingress-sync/commit/0d80ba1eb47a12a2b94bf9ea70d98e516ff6c46f))
* add watch namespace list feature ([ea7a836](https://github.com/rl-io/coredns-ingress-sync/commit/ea7a83661f9a7f262509839d0fa5d42db9d7262a))
* **ci:** add GitHub Actions log grouping to improve test output readability ([c7f444c](https://github.com/rl-io/coredns-ingress-sync/commit/c7f444cca6b086e2add463651512427bc6f2509d))
* enhance namespace watching and pod annotations for deployment ([e1b7fc8](https://github.com/rl-io/coredns-ingress-sync/commit/e1b7fc8209c6543d5458d1f157f5f34dec293be9))
* improve RBAC E2E test debugging and error handling ([4a56c1f](https://github.com/rl-io/coredns-ingress-sync/commit/4a56c1fb3f2e5cf40753b8e12891742687802501))


### Bug Fixes

* **ci:** add actions: write permission to enable repository dispatch ([e236640](https://github.com/rl-io/coredns-ingress-sync/commit/e2366403e0c6bc89f4ad33414d0fd8075b5527a3))
* **ci:** add build, test, and security scan status updates for release-please PRs ([86d1254](https://github.com/rl-io/coredns-ingress-sync/commit/86d125431f9cf8236e99d0ef27d9c5d43c606bca))
* **ci:** add contents: write permission for repository dispatch ([af1df4d](https://github.com/rl-io/coredns-ingress-sync/commit/af1df4dc4f7ee90c737d344dedb8a84abcf766ac))
* **ci:** add job URLs to status updates for better traceability ([31df95a](https://github.com/rl-io/coredns-ingress-sync/commit/31df95a231be26d6f2744e7829d38847de1a4412))
* **ci:** add job-level actions permissions for repository dispatch ([861acc9](https://github.com/rl-io/coredns-ingress-sync/commit/861acc9d7b87a3feb906d541dda536c9245ed4c6))
* **ci:** add missing inputs to security-scan composite action ([27a0232](https://github.com/rl-io/coredns-ingress-sync/commit/27a0232ffbbeada9e111d4b1893a0042e7ca2074))
* **ci:** add missing permissions for statuses in CI/CD workflow ([f1133c7](https://github.com/rl-io/coredns-ingress-sync/commit/f1133c769b95091e88e759574841242fcf452047))
* **ci:** add pending status updates for build, test, and security scan in release-please workflow ([6cc137f](https://github.com/rl-io/coredns-ingress-sync/commit/6cc137f29bcdc4e5625b522ed2e9202789c7a38e))
* **ci:** add setup and update status checks for release-please PRs ([d3bb69b](https://github.com/rl-io/coredns-ingress-sync/commit/d3bb69bae4fccff34b99bca2dd44c1816dee0ab5))
* **ci:** enable build and test trigger for direct pushes to main ([596874a](https://github.com/rl-io/coredns-ingress-sync/commit/596874a668aad27731b38c78662a5c0d53a1cb0f))
* **ci:** enhance job ID retrieval and debugging output in update-pr-status action ([6709db6](https://github.com/rl-io/coredns-ingress-sync/commit/6709db6db4c4641799004b21c619b82933373cd8))
* **ci:** enhance job name mapping and improve job ID retrieval in update-pr-status action ([eb64318](https://github.com/rl-io/coredns-ingress-sync/commit/eb6431801d9fd719baa7cd550ff963590706bb93))
* **ci:** enhance PR status checks with improved SHA retrieval and update actions ([bdfd28e](https://github.com/rl-io/coredns-ingress-sync/commit/bdfd28e893a6941fd6a1718b32eed842121d3f88))
* **ci:** ensure build, test, and security scan jobs always run for CI/CD testing ([116b8f8](https://github.com/rl-io/coredns-ingress-sync/commit/116b8f8884c08759e94901248e42c11d327b0a3a))
* **ci:** go mod tidy ([7f39a3d](https://github.com/rl-io/coredns-ingress-sync/commit/7f39a3d0dbd7dff4e9573b792cd9e7473410ca96))
* **ci:** handle case where kubeconfig not present during unit tests ([22a26f8](https://github.com/rl-io/coredns-ingress-sync/commit/22a26f89e8048cb0f8aacd1d5af25c4c05922f5a))
* **ci:** migrate to simplified workflow ([e217cfe](https://github.com/rl-io/coredns-ingress-sync/commit/e217cfeaf1db3b485206a6aa9d7e56b4c08b47c4))
* **ci:** refine trigger conditions and enhance build/test workflows for repository dispatch events ([4e136b1](https://github.com/rl-io/coredns-ingress-sync/commit/4e136b17e931218ee77766340ab2e243a6c5e1c0))
* **ci:** remove pull_request trigger from CI/CD pipeline and add cicd-changes output to PR tests ([8c06213](https://github.com/rl-io/coredns-ingress-sync/commit/8c0621357e7f016e248f535a26419dee59de8607))
* **ci:** remove redundant cache restore step ([c80352d](https://github.com/rl-io/coredns-ingress-sync/commit/c80352d59e1aa4990f4ca85740591856b04684a7))
* **ci:** retrieve job ID for status updates in update-pr-status action ([d1d4495](https://github.com/rl-io/coredns-ingress-sync/commit/d1d44950210fe977accc1531e75e76438104ab45))
* **ci:** streamline build and test workflows for repository dispatch events ([0d9490e](https://github.com/rl-io/coredns-ingress-sync/commit/0d9490ee5ca7336d5427643ed6418a3a096356dc))
* **ci:** update client-payload in release-please workflow to correctly parse PR number and branch name ([78464f8](https://github.com/rl-io/coredns-ingress-sync/commit/78464f8ddbdf1448dc30e82cbe7e1cc30a113b88))
* **ci:** update condition to trigger CI for release PRs based on prs_created output ([100d9c1](https://github.com/rl-io/coredns-ingress-sync/commit/100d9c12740291d3884ef9ed6737be975385120e))
* **ci:** update release-please to trigger CI via repository_dispatch ([72c0ada](https://github.com/rl-io/coredns-ingress-sync/commit/72c0ada588e873e88b8e95feba9e9cf8cba1beb0))
* **ci:** update status context names in CI/CD workflow for clarity ([e839d63](https://github.com/rl-io/coredns-ingress-sync/commit/e839d63bc09303b452d24ba5d6df42b6bc135f8c))
* create missing test-namespace in RBAC e2e test ([6cf08d9](https://github.com/rl-io/coredns-ingress-sync/commit/6cf08d9d81280b6bd8c6a9d25d6aaa41515d2bd4))
* **deps:** migrate from deprecated dependabot reviewers to CODEOWNERS ([4d191f7](https://github.com/rl-io/coredns-ingress-sync/commit/4d191f7620d863639b742d8dcc9914f420b24ec5))
* remove hardcoded watchNamespaces from controller configuration ([def2f8d](https://github.com/rl-io/coredns-ingress-sync/commit/def2f8dc4fa3f427b8fe2834d8218d22b578720a))
* **test:** create test namespaces before Helm deployment in RBAC E2E test ([887e66d](https://github.com/rl-io/coredns-ingress-sync/commit/887e66de0d45354833a4f1c64a37ffcbc56d4f1e))
* **test:** remove set -e from RBAC E2E test to handle expected failures ([3349537](https://github.com/rl-io/coredns-ingress-sync/commit/334953784f1d76790fe8ce55faf523bedfcda734))
* **test:** skip validation for deleted namespaces in RBAC permissions test ([163f146](https://github.com/rl-io/coredns-ingress-sync/commit/163f146115d2f6f1d949d9a61fbe7ed6ae4a37db))
* update markdown link checker action and correct Cosign documentation link ([71d731f](https://github.com/rl-io/coredns-ingress-sync/commit/71d731f9c8115c72ab36068e8c3d3929d8992b87))
* update namespace watching functionality to allow restricting rbac permissions to specific namespaces or all namespaces for cluster wide permissions ([4fe82ef](https://github.com/rl-io/coredns-ingress-sync/commit/4fe82efe36cf8dc00926ea0edbaecd092445b989))

## [0.1.1](https://github.com/rl-io/coredns-ingress-sync/compare/coredns-ingress-sync-v0.1.0...coredns-ingress-sync-v0.1.1) (2025-07-23)

### Features

* add codecov badge ([68a8187](https://github.com/rl-io/coredns-ingress-sync/commit/68a8187ca34a6f2f5db4956c3630b056057cdb8f))
* add initial project structure and docs ([92aa60a](https://github.com/rl-io/coredns-ingress-sync/commit/92aa60a531df1cf36b2755d976ddaf07525f9464))
* add multi-version local kind tests ([14645c6](https://github.com/rl-io/coredns-ingress-sync/commit/14645c6eaedf2d62b2b15dc63b070254073cddfb))
* add release please for assistance in creating releases ([b64975a](https://github.com/rl-io/coredns-ingress-sync/commit/b64975a1b67a728d5b0508e0b24b8c4f9ea96f77))
* create dependabot.yml ([966d265](https://github.com/rl-io/coredns-ingress-sync/commit/966d2652fc64ac874afddf62c8f61ad297491b89))
* create dependabot.yml ([020c0c1](https://github.com/rl-io/coredns-ingress-sync/commit/020c0c1156f5b439c6b51be670415958f6281bbd))
* **git-hooks:** add conventional commit validation script and update setup script ([0f35642](https://github.com/rl-io/coredns-ingress-sync/commit/0f35642209fa34211b0c99e34a3f3b34e9f360a1))

### Bug Fixes

* **ci:** add release-please multi package configuration ([afb53ff](https://github.com/rl-io/coredns-ingress-sync/commit/afb53ffc02f268304d989176a8aa2fff8c9e69d2))
* **ci:** change release-please type to helm ([a801bb3](https://github.com/rl-io/coredns-ingress-sync/commit/a801bb3d6e921391277f86580054694cc42e2cd6))
* **ci:** correct version search patterns and update manifest path ([78cf8d1](https://github.com/rl-io/coredns-ingress-sync/commit/78cf8d120eefdb6a81492caa7405ace586306932))
* **ci:** fix escaping in release-please config ([086bf9a](https://github.com/rl-io/coredns-ingress-sync/commit/086bf9a48c7005245f97e0653f073f6f392e71d4))
* **ci:** update package path for release-please configuration ([45d794e](https://github.com/rl-io/coredns-ingress-sync/commit/45d794edcb2e80f497dbc642ab8b8ec7eda91a6f))
* **ci:** update readme paths in release-please configuration ([5d753d4](https://github.com/rl-io/coredns-ingress-sync/commit/5d753d4cc34cebc1275022c05a8c9fe98e3a2880))
* **ci:** update release please version pattern matching ([7287950](https://github.com/rl-io/coredns-ingress-sync/commit/72879500fdebacbd18e02c695989ce340abff7a6))
* **ci:** update release-please config for helm and add cli testing ([a6ddcea](https://github.com/rl-io/coredns-ingress-sync/commit/a6ddcea641c4d9afc08b8426acc4fdd90f9a1968))
* **ci:** update release-please configuration for package paths and types ([c67b672](https://github.com/rl-io/coredns-ingress-sync/commit/c67b672697dbe378fd9a2dbc9064bf550c5c8d33))
* publish to /charts directory ([362d9f9](https://github.com/rl-io/coredns-ingress-sync/commit/362d9f98cc4342cb7dad7aaaa5814afc9f15762f))
* release-please versioning fixes ([d482922](https://github.com/rl-io/coredns-ingress-sync/commit/d482922b852d4882329c90a2d3ab8b7a15cb852d))
* **release:** update release-please configuration for helm package ([920de01](https://github.com/rl-io/coredns-ingress-sync/commit/920de0149ff61ac5587d93e98bdc692247a537f6))
* **release:** update release-please configuration to use root package path ([75f1809](https://github.com/rl-io/coredns-ingress-sync/commit/75f1809f46338a59ab1a58d6310ba3a9b890b5bf))
* update codecov actions to use latest ([cfd72ce](https://github.com/rl-io/coredns-ingress-sync/commit/cfd72ce238470e3732893ab8524d9adb236a4be5))
* update github action to use value-dev ([05540a2](https://github.com/rl-io/coredns-ingress-sync/commit/05540a23df7b9eb3cc2c2340833b9f18b69c6ef8))
* update github action to use value-dev ([a3c30fb](https://github.com/rl-io/coredns-ingress-sync/commit/a3c30fb8c91475788836cf828a834225232f95cd))
* update helm release with fallbacks ([300d079](https://github.com/rl-io/coredns-ingress-sync/commit/300d0794380c3e51c37ea18a1f0b31e7633c53e0))
* update release please permissions to allow label creation ([45956b8](https://github.com/rl-io/coredns-ingress-sync/commit/45956b8e26302822eb1b396415399397eb1c9d9c))
* update release-please action and permissions ([393c056](https://github.com/rl-io/coredns-ingress-sync/commit/393c05608299d0878a8b844df3f237470b532d3f))
* update release-please version replacements ([0cf6505](https://github.com/rl-io/coredns-ingress-sync/commit/0cf6505cbcdf2792fe8642103155820d0e780fce))
* update testcleanupfunctions to run correctly ([bcad30e](https://github.com/rl-io/coredns-ingress-sync/commit/bcad30e32587e04c71347e4ac1fdf5d678d3732f))
