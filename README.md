# LANShare 🚀

**LANShare**, Go dili ile geliştirilmiş; Windows, Linux ve macOS işletim sistemlerinde çalışan, aynı yerel ağdaki (LAN) cihazları otomatik keşfeden, modern ve yüksek performanslı dosya paylaşım platformudur. **AirDrop** ve **LANDrop** sadeliğinden esinlenilerek tasarlanmış olup, tek çalıştırılabilir dosya (single executable) olarak dağıtılabilir.

---

## 🇹🇷 Türkçe Dökümantasyon

### ✨ Özellikler

- **🚀 Maksimum Transfer Hızı**: Bellek yönetiminde `sync.Pool` tampon havuzu (Buffer Pool) ve sıfır kopya (Zero-Copy) streaming mimarisi ile yerel ağın izin verdiği maksimum Wi-Fi / Ethernet aktarım hızına ulaşır.
- **⚡ Tek Çalıştırılabilir Dosya (Single Executable)**: Kurulum gerektirmez. Harici kütüphane veya DLL bağımlılığı olmadan tek bir binary dosya olarak çalışır.
- **🌐 Otomatik Cihaz Keşfi (UDP Discovery)**: Yerel ağdaki cihazları UDP broadcast sinyalleri ile otomatik tespit eder. Cihaz adı, işletim sistemi ikonu, IP ve çevrimiçi durumunu liste eder.
- **📂 Sürükle & Bırak ile Dosya ve Klasör Gönderimi**: Dosya veya içi dolu klasör ağaçlarını sürükleyip bırakarak gönderebilirsiniz. Klasörler anlık tar/gzip streaming ile sıkıştırılarak aktarılır.
- **⏸️ Duraklat, Devam Et ve Kaldığı Yerden Sürdür (Resumable Transfers)**: Devam eden transferleri duraklatıp sürdürebilirsiniz. Bağlantı kesintilerinde bayt bazlı `X-Resume-Offset` (Range Resume) protokolü ile dosya sıfırdan başlamaz, kaldığı bayttan devam eder.
- **🔒 Uçtan Uca Şifreleme (AES-256-GCM E2EE)**: İsteğe bağlı olarak transferlerinizi AES-256-GCM şifreleme ile ağ üzerinden güvenli bir şekilde iletebilirsiniz.
- **📊 Anlık Aktarım Metrikleri**: Anlık hız göstergesi (MB/sn), kalan süre (ETA), ilerleme yüzdesi barı ve aktarılan bayt takibi.
- **📜 Geçmiş Transfer Kayıtları**: Gelen ve giden tüm dosyaların geçmişini tutar. Geçmiş listesinden indirilen dosyaların klasörünü tek tıkla (`📁 Klasörü Aç`) açabilirsiniz.
- **🎨 Çoklu Tema ve Dil Desteği**: Koyu ve Açık Tema desteği (Klasik 2000s / Win98 masaüstü estetiği) ile Türkçe ve İngilizce dil seçeneği.

---

### ⚙️ Çalışma Mimarisi ve Teknoloji Detayları

```
+------------------------------------------------------------------------------------+
|                                  LANShare Uygulaması                               |
|                                                                                    |
|  +------------------------+  +------------------------+  +----------------------+  |
|  |     Gömülü Web UI      |  |     Keşif Motoru       |  |    Transfer Motoru   |  |
|  |  (HTML5/CSS3/JS Web-   |  |  (UDP Broadcast /      |  |  (Yüksek Hızlı TCP/  |  |
|  |   Socket REST Router)  |  |   HTTP Prober Beacon)  |  |   HTTP Stream + E2EE)|  |
|  +-----------+------------+  +-----------+------------+  +----------+-----------+  |
|              |                           |                          |              |
|              +---------------------------+--------------------------+              |
|                                          |                                         |
|                               +----------v-----------+                             |
|                               | Go Backend Kontrolcü |                             |
|                               +----------+-----------+                             |
|                                          |                                         |
|                               +----------v-----------+                             |
|                               | Kesintisiz Transfer  |                             |
|                               | & Geçmiş Kayıtçısı   |                             |
|                               +----------------------+                             |
+------------------------------------------------------------------------------------+
```

- **`cmd/lanshare/main.go`**: Web arayüzünü (`go:embed all:web`) tek dosyaya gömer, arka plan servislerini başlatır ve varsayılan tarayıcıyı açar.
- **`pkg/discovery/`**: UDP broadcast dinleyicisi ve dynamic port fallback ile aynı makinede çoklu test desteği sunar.
- **`pkg/transfer/`**: `sync.Pool` 64KB chunk buffer yönetimi, HTTP streaming upload/download, AES-256-GCM şifreleme ve resumable byte offset kontrolü.
- **`pkg/storage/`**: Geçmiş kayıtları JSON formatında saklar (`~/.lanshare/history.json`).
- **`pkg/config/`**: Konfigürasyon yönetimi (`~/.lanshare/config.json`).
- **`pkg/api/`**: REST ve WebSocket canlı durum bildirimi.

---

### 💻 Derleme ve Çalıştırma

#### Gereksinimler
- Go 1.21 veya üzeri.

#### Tek Dosya Derleme (Build)
İşletim sisteminiz için tek binary dosya oluşturmak için:

```bash
go build -ldflags="-s -w" -o lanshare.exe .
```

Tüm platformlar (Windows, Linux, macOS) için `dist/` klasörüne derleme yapmak için:

```bash
# Windows
build.bat

# Linux / macOS
chmod +x build.sh
./build.sh
```

#### Çalıştırma
Oluşan çalıştırılabilir dosyaya çift tıklayabilir veya terminalden çalıştırabilirsiniz:

```bash
./lanshare.exe
```

Tarayıcınız otomatik olarak `http://localhost:52639` adresinde açılacaktır.

---
---

## 🇬🇧 English Documentation

### ✨ Features

- **🚀 Maximum LAN Throughput**: Utilizes zero-copy buffer pooling (`sync.Pool`) and chunked binary streaming to max out your local Wi-Fi / Ethernet bandwidth.
- **⚡ Single Executable (Portable)**: Completely self-contained single binary executable. Requires zero installation, dynamic DLL dependencies, or runtime setup.
- **🌐 Automatic Peer Discovery**: Automatic device discovery using UDP heartbeat beacons across local network subnets. Lists online devices with OS icons, IP, and online status.
- **📂 Drag & Drop File & Folder Transfers**: Drag and drop single files or entire nested folder trees. On-the-fly streaming tar/gzip archive packaging without intermediate disk writes.
- **⏸️ Pause, Resume & Auto-Resume**: Pause and resume active transfers anytime. Interrupted network connections automatically support Range offset resumption from byte positions where connection dropped (`X-Resume-Offset`).
- **🔒 Optional End-to-End Encryption (E2EE)**: Secure payload streams with AES-256-GCM encryption toggleable from settings.
- **📊 Real-Time Metrics**: Live instantaneous transfer speed gauge (MB/s), ETA countdown, percentage progress bar, and byte counter.
- **📜 Complete Transfer History**: Logs all incoming and outgoing transfer records with status badges, timestamps, speeds, and quick "Open Downloads Folder" action.
- **🎨 Dual Themes & Multi-Language**: Dark and Light themes (Classic 2000s / Win98 desktop aesthetic) with Turkish and English language switching.

---

### 💻 Building & Running

#### Prerequisites
- Go 1.21 or higher installed on your system.

#### Build Executable
To build a portable single executable for your current platform:

```bash
go build -ldflags="-s -w" -o lanshare ./cmd/lanshare
```

To cross-compile portable executables for **Windows**, **Linux**, and **macOS**:

```bash
# Windows
build.bat

# Linux / macOS
chmod +x build.sh
./build.sh
```

The output executables will be generated in `dist/`:
- `dist/lanshare-windows.exe`
- `dist/lanshare-linux`
- `dist/lanshare-macos`

#### Run Application
Simply double click or run the executable from terminal:

```bash
./lanshare
```

LANShare will start the local server, display local network details, and automatically open your web browser at `http://localhost:52639`.

---

## 📄 License
MIT License
