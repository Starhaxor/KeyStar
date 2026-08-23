# RFC: Offline Lease ve İmzalı Lisans İş Akışları

**Durum:** Taslak (Sprint D) · **Hedef:** KeyStar çekirdeğine çevrimdışı
lisans doğrulama yeteneği eklemek için tasarım mutabakatı

Bu belge, README yol haritasındaki "Offline lease and signed-license
workflows" maddesinin tasarım temelidir. Mevcut kod tabanındaki yapılar
(Ed25519 `TokenIssuer`, TPM destekli cihaz modeli, refresh rotation,
device policy) üzerine inşa edilir; sıfırdan yeni bir güvenlik katmanı
önermez.

---

## 1. Motivasyon

Bugün her yetkilendirme anlık sunucu doğrulamasına bağlıdır: access token
kısa ömürlüdür, refresh token yalnızca online iken döner. Aşağıdaki
senaryolar bu modeli kırar:

1. **Ağ kesintisi / kısıtlı bağlantı:** Fabrika sahası, gemi, uçak, saha
   ekipmanı — lisanslı kullanıcı günlerce offline kalabilir.
2. **Sunucu erişilemezliği:** Self-hosted KeyStar bakım moduna alındığında
   tüm istemcilerin çalışmayı durdurması ticari olarak kabul edilemez.
3. **Yüksek gecikmeli telemetri:** Her başlangıçta zorunlu token yenileme,
   mobil/kotalı bağlantıda kötü deneyim üretir.

Cryptolens ve LicenseSpring'in olgun alanları tam da budur; KeyStar'ın
farkı, offline doğrulamayı **cihaz güven modeliyle (TPM) ve uygulama
izolasyonuyla birlikte** tasarlamaktır.

## 2. Hedefler ve Hedef Olmayanlar

**Hedefler**

- İstemci, sunucuya hiç ulaşmadan lisans durumunu kriptografik olarak
  doğrulayabilmeli.
- Offline süre, uygulama bazında politikayla sınırlandırılmalı
  (varsayılan yok — kapalı başlar).
- İptal (revoke) en geç policy penceresi sonunda etkin olmalı.
- Cihaz bağlaması zayıflatılmamalı; lease tek bir kayıtlı cihaza bağlı.
- Mevcut online akış hiç değişmeden çalışmaya devam etmeli.

**Hedef olmayanlar**

- Tam kopuk "kalıcı lisans dosyası" satışı (perpetual) — ileri faz.
- Paylaşılan/taşınabilir lisans dosyası desteği — amaç dışı; lease
  tekil cihaz+kullanıcı çiftine bağlıdır.

## 3. Tehdit Modeli

| Saldırı | Savunma |
| --- | --- |
| Lease dosyasının kopyalanıp başka makinede kullanımı | Lease claim'leri cihaz kimliğine bağlı (`device_id` + TPM fingerprint hash); SDK açılışta yerel TPM kanıtını claim ile eşler |
| Ed25519 public key değiştirme | Doğrulama anahtarı istemci paketine gömülü; sunucu anahtarı rotasyonu iki key-ID (`kid`) destekli yayınla yapılır |
| Saat geriye alınması → süresiz offline | Monotonik checkpoint: SDK, gördüğü en büyük zamanı HMAC'li yerel kayıtta tutar; `lease.iat > checkpoint` ise lease reddedilir |
| Lease içindeki hakların (features/level) genişletilmesi | İmza tüm claim setini kapsar; private key yalnızca sunucuda |
| İptal edilen lisansın grace bitene kadar çalışması | Bilinçli ticari taviz: pencere üst sınırı politika ile sınırlanır; online temas anında iptal derhal uygulanır |

## 4. Tasarım

### 4.1 Lease Token

Lease, mevcut `TokenIssuer` ile imzalanan ayrı bir JWT türüdür. Yeni
claim seti:

```json
{
  "iss": "keystar", "aud": "keystar-clients",
  "typ": "offline-lease",
  "sub": "<user_id>",
  "app":  "<application_id>",
  "dev":  "<device_id>",
  "tpm":  "<sha256(tpm_public_key)>",
  "lic":  { "license_id": "...", "plan": "...", "level": 3,
            "features": ["pro"], "max_devices": 2 },
  "grace_until": "2026-10-01T00:00:00Z",
  "iat": ..., "nbf": ..., "exp": ...
}
```

- `exp` = kısa dönem (örn. 24 saat): online istemci sessizce yeniler,
  token hırsızlığı yüzeyi küçük kalır.
- `grace_until` = uzun dönem (`exp` ≤ `grace_until` ≤ policy limiti):
  sunucu hiçbir zaman erişilemezse bile istemcinin kesin duracağı tavan.

### 4.2 Politika (Device Policy'nin uzantısı)

Mevcut `device_policies` tablosuna sütunlar eklenir (migration):

```sql
alter table device_policies
    add column offline_enabled boolean not null default false,
    add column offline_max_hours integer not null default 0;
```

- `offline_enabled=false` (varsayılan): bugünkü davranış, hiçbir değişiklik yok.
- `offline_max_hours`: `grace_until` için üst sınır (örn. 72h–2160h).

### 4.3 API Yüzeyi

| Uç | Davranış |
| --- | --- |
| `POST /v1/device/lease` (publishable + mevcut session) | Aktif lisans + verified device varsa lease imzalar; yoksa 403. Rate limiter'a tabii. |
| `/v1/me` yanıtına `offline` bloğu | Kalan grace, yenileme zamanı — SDK görünürlüğü. |

İptal/revoke, bir sonraki online temasta normal akışla işler; lease
yenilenemez ve `grace_until`'e kadar yaşamaya devam eder (bilinçli taviz,
§3).

### 4.4 İstemci (SDK) Etkisi

- `token_store` lease'i saklar; açılışta: local verify (embedded public
  key) → TPM fingerprint eşleşmesi → checkpoint kontrolü → geçerliyse
  offline mod.
- Checkpoint: HMAC(LICENSE_HMAC_KEY türevi, last_seen_ts) disk/TPM NV'de.
- Qt/C++ adapter'larında "offline since" UI sinyali önerilir.

### 4.5 Gözlemlenebilirlik

- Metrik: `keystar_leases_issued_total{application}` (mevcut metrics paketi).
- Audit: `LEASE_ISSUED` action'ı; konsol Audit Log'da filtrelenir
  (mevcut server-side search bunu zaten karşılar).

## 5. Faz Planı

| Faz | İçerik | Çıktı |
| --- | --- | --- |
| D1 | Migration (policy sütunları) + issuer'a `typ: offline-lease` desteği | Şema + birim testler |
| D2 | `POST /v1/device/lease` + policy enforcement + audit/metrics | API + entegrasyon testleri |
| D3 | C++ SDK: lease saklama, checkpoint, offline doğrulama akışı | SDK sürümü |
| D4 | Konsol: application ayarlarına offline politikası alanı | UI |

Her faz bağımsız dağıtılabilir; D2 öncesi hiçbir davranış değişmez.

## 6. Açık Sorular (karar bekliyor)

1. Varsayılan `exp` süresi 24h mı, 12h mi? (Token yüzeyi vs. yenileme trafiği)
2. Grace penceresinde feature-level kısıtlama (örn. salt-okunur mod)
   gerekir mi, yoksa tam fonksiyon mu?
3. Birden çok aktif cihazda aynı anda offline lease — toplam offline
   kota uygulanmalı mı?
4. Lease iptali için merkezi "kill list" diff senkronu gerekecek mi
   (uzun grace kullanan müşteriler için)?

Kararlarla birlikte bu RFC, D1 migration'ın tasarım girdisi olur.
