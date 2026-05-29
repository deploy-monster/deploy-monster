# DeployMonster Self-Hosted Readiness Raporu

Tarih: 2026-05-26

Bu rapor, DeployMonster'ın self-hosted kurulumu, tek sunucu çalışma modu ve çoklu sunucu/agent modu için mevcut durumunu, bu turda yapılan düzeltmeleri ve üretim öncesi kapatılması gereken riskleri özetler.

## Yönetici Özeti

DeployMonster bugün tek binary ile master veya agent olarak çalışabilecek mimariye sahip. Tek sunuculu self-hosted kurulum yolu pratik olarak kullanılabilir seviyede: Go API, gömülü React arayüz, SQLite/Postgres store arayüzü, ingress, deploy trigger, auth, domain label üretimi ve installer akışı aynı binary üzerinden çalışıyor.

Çoklu sunucu tarafında agent protokolü, master tarafında agent WebSocket endpoint'i ve remote executor arayüzleri zaten vardı. Bu çalışmada bu parçalar API ve deploy akışına bağlandı: bağlı agent'lar artık `/api/v1/agents` üzerinden gerçek remote agent olarak görünebiliyor ve `Application.ServerID` dolu olduğunda image deploy ile build sonrası git deploy container başlatma işlemi hedef agent'a yönlenebiliyor.

Kritik gerçek: çoklu sunucu modu artık sadece "protokol var ama uygulama deploy yolu kullanmıyor" seviyesinde değil. İlk remote placement yolu açıldı, API/UI hedef server seçimini taşıyabiliyor, remote agent üzerinde deploy network'ü idempotent şekilde hazırlanabiliyor ve remote git deploy için registry push/pull yolu konfigüre edilebiliyor. Yine de tam üretim kalitesinde çoklu sunucu için per-app registry policy, remote build scheduling, ingress/topology kararı ve staging ortamında uçtan uca kanıt hâlâ tamamlanması gereken parçalardır.

## Yapılan Değişiklikler

### Agent servisinin core'a bağlanması

Önceden `swarm.AgentServer` master tarafında bağlantıları kabul ediyordu fakat `core.Services` içinde genel servis olarak expose edilmiyordu. Bu nedenle API handler'ları ve deploy akışı bağlı agent listesini veya agent executor'larını doğrudan kullanamıyordu.

Yapılanlar:

- `core.Services` içine `Nodes core.NodeManager` alanı eklendi.
- `swarm.Module.Init` sırasında `c.Services.Nodes = m.agentServer` atanıyor.
- Böylece master process içinde agent registry, API ve deploy handler'ları tarafından ortak servis olarak erişilebilir hale geldi.

Sonuç:

- Master artık bağlı agent'ları sadece swarm modülünün içinde tutmuyor.
- API, agent listesi ve remote deploy placement aynı NodeManager üzerinden çalışabiliyor.

### Agent API gerçek remote agent'ları göstermeye başladı

Önceden `/api/v1/agents` endpoint'i pratikte local node bilgisini dönüyordu; çoklu sunucu kurulumunda bağlı remote agent'lar kullanıcı/API açısından görünür değildi.

Yapılanlar:

- `AgentStatusHandler` NodeManager destekliyorsa `ConnectedAgents()` çıktısını okuyor.
- Her remote agent için hostname, IP, version, last seen ve varsa metrics bilgileri dönüyor.
- Ping başarısız olursa agent unhealthy olarak işaretleniyor.
- `GET /api/v1/agents/{id}` bağlı remote agent'ı dönebiliyor; yoksa 404 dönüyor.
- Remote metrics/ping çağrıları için kısa timeout kullanılıyor.

Sonuç:

- Multi-server master üzerinde bağlı agent'lar API'de gözlemlenebilir hale geldi.
- UI veya dış otomasyonlar `/api/v1/agents` üzerinden gerçek cluster görünürlüğü alabilir.

### Deploy akışı `Application.ServerID` ile remote placement yapabiliyor

Önceden deploy trigger handler container başlatma için sadece local `core.ContainerRuntime` kullanıyordu. Uygulama bir server'a atanmış olsa bile deploy işlemi remote agent'a yönlenmiyordu.

Yapılanlar:

- `DeployTriggerHandler` içine `core.NodeManager` desteği eklendi.
- `SetNodeManager` router'dan çağrılıyor.
- `deployRuntimeForApp` eklendi:
  - `app.ServerID == ""` veya `local` ise local runtime kullanılır.
  - `app.ServerID` remote bir agent ID ise NodeManager üzerinden agent executor alınır.
  - Agent bağlı değilse deploy failed olur.
- Image tabanlı deploy'da container oluşturma ve eski container temizliği hedef runtime üzerinde yapılır.
- Git deploy'da build hâlâ master/local runtime ile yapılır; build sonucu image container'ı hedef agent üzerinde başlatılabilir.
- Webhook deploy ve manual git deploy aynı `deployGitApp` yolunu kullanacak şekilde toplandı.
- Deploy sonrası `app.deployed` event'i image deploy için de container ID ile yayınlanıyor.
- Eski app container'ları, yeni container başarılı başladıktan sonra aynı hedef runtime üzerinde temizleniyor.
- Deploy öncesi hedef runtime üzerinde `monster-network` hazırlanıyor.
- Remote git deploy için build sonucu registry-qualified image değilse deploy erken failed oluyor. Bu, master'da lokal üretilen `monster/...` image'ın agent tarafından yanlışlıkla Docker Hub'dan çekilmeye çalışılmasını veya yanlış image ile deploy edilmesini engeller.
- `docker.build_image_registry` veya `MONSTER_BUILD_IMAGE_REGISTRY` ayarlıysa remote git build image tag'i baştan registry-qualified üretilir.
- `docker.build_image_push` veya `MONSTER_BUILD_IMAGE_PUSH=true` ayarlıysa build tamamlandıktan sonra `docker push <image>` çalışır.
- `docker.build_registry_username/password` veya `MONSTER_BUILD_REGISTRY_USERNAME/PASSWORD` ayarlıysa build/push geçici `DOCKER_CONFIG` ile bu credential'ları kullanır; runtime image pull da aynı credential'ları Docker Registry Auth header olarak gönderir.

Sonuç:

- `Application.ServerID = agent-1` olan image app deploy edildiğinde container agent üzerinde başlatılabilir.
- Git app için build master tarafında tamamlandıktan sonra container remote agent üzerinde başlatılabilir.
- Agent yoksa deploy "başarılıymış gibi" görünmez; app failed durumuna alınır.
- Remote agent üzerinde `monster-network` yoksa deploy akışı `network.create` protokol komutuyla bunu oluşturmaya çalışır.
- Remote git deploy'un başarılı olabilmesi için build sonucunun agent tarafından erişilebilen registry-qualified image tag olması gerekir.
- Örnek: `MONSTER_BUILD_IMAGE_REGISTRY=ghcr.io/acme/deploymonster` ayarı remote git deploy için `ghcr.io/acme/deploymonster/<app>:<tag>` formatında image üretir.
- Örnek push: `MONSTER_BUILD_IMAGE_PUSH=true` ile master build bittikten sonra image'ı registry'ye push eder; agent container create sırasında aynı image'ı pull eder.
- Örnek auth: `MONSTER_BUILD_REGISTRY_USERNAME=deploymonster` ve `MONSTER_BUILD_REGISTRY_PASSWORD=<token>` ayarları push sırasında geçici Docker config olarak, pull sırasında Docker Registry Auth header olarak kullanılır.

### Agent protokolünde network create bağlandı

Önceden `AgentMsgNetworkCreate` protokol sabiti vardı ancak agent client bu mesajı işlemiyor, remote executor da network create komutu göndermiyordu.

Yapılanlar:

- `NodeExecutor` arayüzüne `EnsureNetwork(ctx, name)` eklendi.
- `RemoteExecutor.EnsureNetwork` master'dan agent'a `network.create` mesajı gönderiyor.
- `AgentClient` `network.create` mesajını alıp local runtime `EnsureNetwork` destekliyorsa çağırıyor.
- `LocalExecutor` da runtime destekliyorsa aynı metodu çağırıyor.
- Deploy handler, container create öncesi hedef runtime üzerinde `monster-network` ensure ediyor.

Sonuç:

- Remote deploy artık "network yok" durumunu container create sırasında sürpriz hata olarak yaşamıyor.
- Agent runtime network ensure desteklemiyorsa hata net şekilde master'a dönüyor ve deploy failed oluyor.

### VPS provider kayıtları token tabanlı hale geldi

Önceden VPS provider modülü provider isimlerini log'luyordu ancak gerçek provider token konfigürasyonu core config/env akışına bağlı değildi.

Yapılanlar:

- `VPSProvidersConfig` içine provider token alanları eklendi:
  - `hetzner_token`
  - `digitalocean_token`
  - `vultr_token`
  - `linode_token`
- Ortam değişkenleri eklendi:
  - `MONSTER_HETZNER_TOKEN`
  - `MONSTER_DIGITALOCEAN_TOKEN`
  - `MONSTER_VULTR_TOKEN`
  - `MONSTER_LINODE_TOKEN`
- Secret audit bu token'ları hassas değer olarak tanıyor.
- `vps.Module` sadece token'ı olan cloud provider'ı register ediyor.
- Custom provider her zaman register ediliyor.
- `RegisterVPSProvisioner` nil map durumunda güvenli şekilde initialize ediyor.

Sonuç:

- `/api/v1/servers/providers` gibi provider kullanan API yüzeyleri artık config ile gerçek provider availability gösterebilir.
- Self-hosted kullanıcı sadece kullandığı cloud token'ını vererek provider'ı açabilir.

### Installer ve dokümantasyon düzeltildi

Önceden installer ve dokümanlarda platform UI/API HTTPS ve port varsayımları karışıktı. Domain verilince platform port'unun 443'e taşınması gibi gerçek kurulumda kırılabilecek bir drift vardı.

Yapılanlar:

- Platform UI/API varsayılan olarak `http://<domain>:8443` şeklinde dokümante edildi.
- Application ingress için 80/443 ayrı anlatıldı.
- Installer `MONSTER_SECRET` kullanıyor, geriye uyumluluk için `MONSTER_SECRET_KEY` fallback'i var.
- CORS origin `*` yerine kurulum parametrelerinden türetiliyor.
- Versiyon örnekleri `v0.1.8` olarak güncellendi.
- Agent endpoint dokümantasyonda `/api/v1/agent/ws` olarak düzeltildi.
- Multi-server bölümünde master join token ve agent başlatma komutu netleştirildi.
- Installer `--agent --master=<url> --token=<join-token> --server-id=<id>` parametrelerini destekliyor.
- Agent kurulumunda systemd unit `deploymonster serve --agent` komutunu çalıştırıyor.
- Agent master URL/join token/server ID değerleri `/etc/deploymonster/deploymonster.env` içine yazılıyor.
- Master tarafında `MONSTER_JOIN_TOKEN` artık `swarm.join_token` override'ı olarak uygulanıyor; installer env veya `--token` değerini master systemd env dosyasına persist edebiliyor.
- Agent mode `--token`, `--agent-cert`, `--agent-key`, `--agent-ca` değerleri verilmezse `swarm.join_token` ve `swarm.tls_*` config değerlerine fallback yapıyor; aynı değerler `MONSTER_AGENT_CERT_FILE`, `MONSTER_AGENT_KEY_FILE`, `MONSTER_AGENT_CA_FILE` ile override edilebiliyor.

Sonuç:

- Self-hosted kurulum talimatları binary'nin gerçek davranışına daha yakın.
- Worker node kurulumu aynı installer ile tekrarlanabilir hale geldi.
- Platform API/UI ile deployed app ingress ayrımı netleşti.

## Sistem Bugün Nasıl Çalışıyor?

### Tek sunucu modu

Tek sunucu modunda `deploymonster serve` master olarak çalışır. Aynı process içinde:

- API router açılır.
- Auth/JWT/API key kontrolü yapılır.
- Store arayüzü üzerinden uygulama, deployment, domain, server ve diğer kayıtlar yönetilir.
- Local container runtime ile image veya git build sonucu container başlatılır.
- Domain bilgileri container label'larına çevrilir.
- React arayüz Go binary içine gömülü static dosyalardan servis edilir.

Kurulum açısından kritik build sırası:

1. `web` dizininde React build alınır.
2. `web/dist` içeriği `internal/api/static` içine kopyalanır.
3. Go binary build edilir.

Bu yüzden self-hosted release üretiminde `scripts/build.sh` tercih edilmelidir. Sadece `make build` çalıştırmak UI static asset'lerinin güncel olduğunu varsayar.

### Çoklu sunucu master/agent modu

Master tarafı:

- `swarm.Module` `/api/v1/agent/ws` endpoint'ini register eder.
- Join token ile gelen agent bağlantısını kabul eder.
- Agent bağlantıları `NodeManager` olarak core services'e bağlanır.
- API handler'ları bağlı agent listesini okuyabilir.
- Deploy handler, uygulamanın `ServerID` değerine göre local veya remote runtime seçer.

Agent tarafı:

- `deploymonster serve --agent --master=http://<master-host>:8443 --token=<shared-secret>` ile master'a bağlanır.
- Master'dan gelen executor komutlarını agent üzerinde uygular.
- Metrics, ping ve container işlemleri master tarafından çağrılabilir.

Remote deploy davranışı:

- `ServerID` boşsa local deploy yapılır.
- `ServerID=local` ise local deploy yapılır.
- `ServerID=<agent-id>` ise container create/start ve cleanup işlemi o agent'a gönderilir.
- Agent bağlı değilse deploy failed olur.

### Uygulama hedef server seçimi API ve UI'a bağlandı

Remote deploy kararını veren alan `core.Application.ServerID`. Deploy handler bu alanı okuyordu, ancak uygulama oluşturma/güncelleme API'leri önceki durumda `server_id` payload alanını kabul etmiyordu. Bu nedenle remote placement kodu var olsa bile gerçek create/update akışında hedef node seçimi kaybolabiliyordu.

Yapılanlar:

- `POST /api/v1/apps` payload'ına `server_id` eklendi.
- `PATCH /api/v1/apps/{id}` payload'ına pointer tabanlı `server_id` eklendi; boş string gönderilirse remote placement temizlenebiliyor.
- `server_id` için 100 karakter sınırı ve güvenli karakter doğrulaması eklendi.
- SQLite ve Postgres `UpdateApp` artık `server_id` alanını da persist ediyor.
- SQLite ve Postgres app liste sorguları `server_id` alanını da döndürüyor.
- `/api/v1/servers` artık kayıtlı server status'undan ayrı olarak canlı agent bağlantısını `connected` ve `agent_status` alanlarıyla gösteriyor.
- App create/update API'leri `server_id` değerinin `local`, tenant'a açık kayıtlı bir server veya canlı bağlı bir agent olmasını zorunlu kılıyor.
- Frontend API tipleri `server_id` alanını içeriyor.
- Deploy wizard `/servers` listesinden hedef server seçebiliyor; varsayılan hedef local server.
- Deploy wizard ve app detail disconnected agent'ları hedef olarak seçtirmiyor.
- App detail ayarlar ekranı mevcut app'in hedef server'ını değiştirebiliyor veya local server'a geri alabiliyor.
- Compose stack ve marketplace deploy API'leri de `server_id` kabul edip aynı hedef validasyonunu uyguluyor; stack servisleri seçilen agent executor üzerinde başlatılabiliyor.
- Marketplace listesi ve template detail ekranları hedef server seçimini deploy payload'ına taşıyor.
- OpenAPI şeması create/update/App modelinde `server_id` alanını gösteriyor.

Sonuç:

- Remote placement artık sadece iç model veya test datası ile değil, normal API ve UI create akışından da seçilebilir.
- API istemcileri rastgele veya başka tenant'a ait `server_id` yazarak deploy hedefini kirletemez; hedef bulunamazsa request validation error ile reddedilir.
- Bir app sonradan local'e geri alınabilir veya başka agent'a taşınmak üzere `server_id` güncellenebilir.
- Kayıtlı ama agent bağlantısı olmayan server'lar UI'da deploy-ready gibi görünmez; bu, hedef seçilip deploy sırasında sonradan patlayan akışı azaltır.

### Staging smoke testi multi-server kontrolleri taşıyor

Önceden `scripts/staging-smoke.sh` public health, auth ve temel app list kontrolü yapıyordu. Multi-server hedefi için bu yeterli değil; agent bağlantısının ve remote deploy tetiklemesinin staging ortamında ayrı kanıtlanması gerekiyor.

Yapılanlar:

- Authenticated smoke içine `/api/v1/agents` kontrolü eklendi.
- `DM_SMOKE_REQUIRE_AGENT=1` ile en az bir remote agent zorunlu tutulabiliyor.
- `DM_SMOKE_AGENT_ID=<id>` ile spesifik agent bağlantısı aranabiliyor.
- `DM_SMOKE_REMOTE_SERVER_ID=<id>` ve `DM_SMOKE_REMOTE_IMAGE=<image>` verilirse smoke script remote target'lı image app oluşturup deploy tetikliyor.
- Remote smoke app cleanup için `DELETE /api/v1/apps/{id}` çağrısı yapılıyor.

Sonuç:

- "Master ayakta mı?" seviyesi ile "master+agent remote deploy yolu çalışıyor mu?" seviyesi ayrı smoke modlarıyla ölçülebilir hale geldi.

## Test Edilenler

Bu çalışma sonunda şu doğrulamalar geçti:

```bash
go test ./internal/api/handlers -run 'TestDeployTrigger|TestAgentStatus'
go test ./internal/api/handlers -run 'TestCreateApp|TestAppUpdate|TestUpdateApp|TestDeployTrigger'
go test ./internal/db -run 'TestSQLite.*App|TestPostgresDB_.*App|TestSQLiteCoverage_App_UpdateApp|TestStoreContract'
go test ./internal/api ./internal/api/handlers ./internal/swarm ./internal/core ./internal/auth ./internal/vps ./internal/build ./internal/deploy ./internal/db ./cmd/deploymonster
cd web && pnpm run build
go run ./cmd/openapi-gen
bash -n scripts/install.sh scripts/staging-smoke.sh
git diff --check
```

Eklenen/iyileştirilen test kapsamı:

- Remote agent listesi `/api/v1/agents` cevabında görünüyor.
- Remote agent detail endpoint'i bağlı agent'ı döndürüyor.
- Image app remote server'a deploy ediliyor.
- Remote server yoksa deploy failed oluyor.
- Git app build sonrası remote server üzerinde container başlatıyor.
- Remote deploy öncesi `monster-network` ensure ediliyor.
- Agent client `network.create` mesajını runtime `EnsureNetwork` çağrısına çeviriyor.
- Remote git deploy registry-qualified olmayan build image'larını reddediyor.
- Remote git deploy için build image registry prefix'i config/env ile üretilebiliyor.
- Remote git deploy için build sonrası optional image push config/env ile açılabiliyor.
- App create/update `server_id` validasyonu kayıtlı tenant server'ı, shared server, canlı agent, eksik server ve cross-tenant server senaryolarını kapsıyor.
- Compose stack ve marketplace deploy `server_id` validasyonu ve remote executor seçimi test edildi.
- Agent CLI config fallback'i, `swarm.join_token` ve `swarm.tls_*` env override'ları test edildi.
- Remote git build/push registry credential'ları config/env üzerinden geçici Docker config'e bağlanabiliyor.
- Master ve agent Docker runtime image pull işlemleri aynı registry credential'larını kullanabiliyor.
- App create/update API'leri `server_id` alanını doğrulayıp persist ediyor.
- Deploy wizard hedef server seçimini create payload'ına taşıyor.
- App detail hedef server değişikliğini update payload'ına taşıyor.
- Server listesi kayıtlı server ile canlı agent bağlantısını ayrıştırıyor.
- Staging smoke script agent varlığı ve remote image deploy tetikleme modlarını destekliyor.
- Git deploy build sonucu container ID deployment kaydına yazılıyor.
- Webhook deploy manual git deploy ile aynı core deploy yolunu kullanıyor.
- VPS modülü sadece token'ı olan cloud provider'ları register ediyor.

## Üretime Hazırlık Durumu

### Hazır veya büyük ölçüde hazır

- Tek binary master modu.
- Embedded frontend build modeli.
- Tek sunucu image deploy.
- Git build ve deploy akışı.
- Domain label üretimi.
- Deploy freeze kontrolü.
- Agent WebSocket endpoint'i.
- Agent visibility API.
- İlk remote placement akışı.
- Token tabanlı VPS provider registration.
- Installer config drift'inin önemli bölümü.

### Kısmen hazır

- Multi-server deploy:
  - Remote container start yolu var.
  - Remote cleanup yolu var.
  - Remote network ensure yolu var.
  - Agent görünürlüğü var.
  - API ve deploy wizard hedef server seçimini taşıyabiliyor.
  - Ancak ingress/topology kararı ve staging kanıtı tamamlanmalı.

- Git deploy remote:
  - Build master tarafında yapılıyor.
  - Container remote agent'ta başlatılabiliyor.
  - Registry-qualified olmayan build image'ları remote deploy'da reddediliyor.
  - `docker.build_image_registry` ayarlanırsa remote git build için registry-qualified image tag kullanılıyor.
  - `docker.build_image_push` ayarlanırsa build sonrası image registry'ye push ediliyor.
  - Build registry username/password ayarlanırsa build/push geçici Docker config ile, pull ise Docker Registry Auth header ile auth kullanıyor.
  - Tenant-scoped registry credential seçimi ve per-app registry policy ayrıca tamamlanmalı.

- VPS provisioning:
  - Provider registration config'e bağlandı.
  - Provider implementasyonlarının gerçek cloud API davranışı staging'de ayrıca doğrulanmalı.

### Üretim öncesi kritik kalan işler

1. Remote image availability:
   - Registry-qualified tag üretimi ve optional `docker push` yolu var.
   - Push ve pull auth env/config ile verilebilir veya Docker host'larında önceden yapılandırılabilir.
   - Alternatif: image save/load stream protocol, fakat bu daha ağır ve hata yüzeyi daha geniş.
   - Şu an yanlış deploy'u önlemek için remote git deploy registry-qualified olmayan image tag'larını reddeder.
   - Kalan ürün kararı: registry credential'ları global config'ten mi, tenant/app scope'tan mı seçilecek?

2. Scheduling policy:
   - Bugün hedef `Application.ServerID` ile explicit seçiliyor.
   - Otomatik placement isteniyorsa capacity, health, labels, region ve resource score kullanan scheduler gerekir.

3. Agent reconnect ve in-flight deploy davranışı:
   - Deploy sırasında agent koparsa status transition ve retry policy netleşmeli.
   - "unknown", "failed", "retryable failed" gibi durumlar ayrılmalı.

4. Multi-server ingress/topology:
   - Container remote agent'ta çalışınca traffic'in hangi node üzerinden geleceği net olmalı.
   - Tek ingress master mı, her node ingress mi, yoksa external LB mi kullanılacak karar verilmeli.

5. Installer smoke test:
   - Temiz Ubuntu VM üzerinde install script uçtan uca çalıştırılmalı.
   - Domain'li ve domain'siz kurulum ayrı denenmeli.
   - Re-run idempotency testi yapılmalı.

6. Release artifact doğrulaması:
   - `scripts/build.sh` ile binary üretimi.
   - Embedded UI asset'lerinin binary içinde doğru servis edildiği smoke test.
   - OpenAPI drift ve CI local quick gate.

## Önerilen Sonraki Teknik Sıra

1. Git build output için registry push/pull stratejisini netleştir ve uygula.
2. Remote deploy için staging smoke testi yaz:
   - master container/process
   - agent container/process
   - image app remote deploy
   - git app remote deploy
   - agent disconnect failure case
3. Server create/provision akışından agent join token bootstrap'ına kadar uçtan uca VPS testi yap.
4. Installer için temiz VM test matrisi oluştur.

## Sonuç

Self-hosted tek sunucu kurulumu için ana kırılma noktaları azaltıldı ve dokümantasyon gerçek çalışmaya yaklaştırıldı. Çoklu sunucu için daha önce kopuk duran agent registry, API görünürlüğü ve deploy placement yolu birbirine bağlandı. Bu, multi-server hedefi için önemli bir eşik.

Ancak "sıfır sorun self-hosted multi-server" diyebilmek için registry credential kapsamı, ingress/topology kararı ve master+agent staging smoke testinin kanıtlanması gerekiyor. Bugünkü durum tek sunucuda kullanılabilir, çoklu sunucuda ise API/UI'dan hedef server seçilebilen ve registry tabanlı image dağıtımı konfigüre edilebilen ilk işlevsel remote placement seviyesidir.
