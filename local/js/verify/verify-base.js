// 3D Artefact Exhibition server
// File: verfiy-scan.js, Ancient Artefacts Authentication - verify method script
// Creater: Kevin Mak, October 2021
// (c)Marvel Digital Ltd. 2021

const Index = {
  template: `
<q-layout view="hHh LpR fFf">
  <q-header elevated class="bg-primary text-white">
    <q-toolbar>
      <q-btn dense flat round icon="home" to="/"></q-btn>
      <q-toolbar-title v-model="">
        {{$t("appTitle")}}
      </q-toolbar-title>
    </q-toolbar>
  </q-header>

  <q-page-container>
    <router-view></router-view>
  </q-page-container>
</q-layout>`,
  setup() {
    return {};
  },
};

const Home = {
  template: `
<transition appear name="fade" mode="out-in">
<q-page class="flex flex-center q-pa-md">
  <div class="flex flex-center q-gutter-xl column">
  <!-- @click="nfcWarning = true" -->
    <q-btn :disable="valueData == null" class="col" color="white" text-color="black" size="xl" to="/nfc">
      <q-icon name="nfc" class="text-primary"></q-icon>
      <span v-if="valueData == null" style="text-decoration: line-through;" class="q-pl-sm">{{$t("nfcTitle")}}</span>
      <span v-else class="q-pl-sm">{{$t("nfcTitle")}}</span>
    </q-btn>
    <q-btn :disable="true" class="col" color="white" text-color="black" size="xl" to="/ai">
      <q-icon name="camera" class="text-green"></q-icon>
      <span class="q-pl-sm" style="text-decoration: line-through;">{{$t("aiTitle")}}</span>
    </q-btn>
    <q-btn :disable="true" class="col" color="white" text-color="black" size="xl" to="/custom">
      <q-icon name="build" class="text-yellow"></q-icon>
      <span class="q-pl-sm" style="text-decoration: line-through;">{{$t("customTitle")}}</span>
    </q-btn>
  </div>

  <q-dialog v-model="nfcWarning">
    <q-card class="bg-warning">
      <q-card-section >
        <div class="text-h5 text-white">Warning</div>
      </q-card-section>

      <q-card-section class="text-white">
        {{$t("nfcWarning")}}
      </q-card-section>

      <q-card-actions align="right">
        <q-btn class="text-white" label="OK" color="yellow-10" v-close-popup to="/nfc"></q-btn>
      </q-card-actions>
    </q-card>
  </q-dialog>
</q-page>
</transition>`,
  data() {
    return {
      valueData: inputData,
      nfcWarning: false,
    };
  },
  mounted() {
    VueRouter.useRouter().push('/nfc');
  },
};

const nfc = {
  template: `<transition appear name="fade" mode="out-in">
<q-page class="q-mb-xl">
  <div class="flex flex-center">
    <div v-if="valueData == 'OK'" class="flex flex-center column">
      <h4 class="col text-positive">{{$t("verifyTitle")}}</h4>
      <div v-if="ModelName.length > 0" class="col q-ma-md text-h5">{{ModelName}}</div>
      <q-img v-if="ModelImg.length > 0" :src="ModelImg" spinner-color="white"></q-img>
      <q-btn v-if="ModelLink.length > 0" type="a" :href="ModelLink" color="primary">{{$t("toNFClib")}}</q-btn>
    </div>
    <div v-else class="flex flex-center column">
      <h4 class="col text-negative">{{$t("verifyFailTitle")}}</h4>
      <div class="col text-body1">{{$t("verifyFailMessage")}}</div>
    </div>
  </div>
</q-page>
</transition>`,
  data() {
    return {
      valueData: inputData,
      infoLoadOk: false,
      link: linkId,
      ModelName: "",
      ModelImg: "",
      ModelLink: "",
    };
  },
  mounted() {
    axios
      .get("/verify/api/linkmodel/" + this.link)
      .then((r) => {
        if (r.data.msg == "OK") {
          this.infoLoadOk = true;
          this.ModelImg = r.data.img;
          this.ModelName = r.data.name;

          /** @type {string} */
          let link = r.data.desc;
          let baseLink = "https://www.coinllectibles.art/";
          if (link.startsWith(baseLink)) {
            let after = link.substring(31);
            if (link === baseLink) after = "home";
            if (after.startsWith("cn") || after.startsWith("en")) {
              this.ModelLink = link;
              return;
            }

            if (i18n.global.locale.includes("zh")) {
              this.ModelLink = baseLink + "cn/" + after;
            } else {
              this.ModelLink = baseLink + "en/" + after;
            }
          } else {
            this.ModelLink = link;
          }
        }
      })
      .catch((e) => {});
  },
};
const ai = {
  template: `
<transition appear name="fade" mode="out-in">
<q-page>
  <div style="font-size: 48px" class="flex flex-center">AI</div>
  <div class="flex flex-center q-ma-md">
    <q-btn size="md" color="primary" @click="customQR()" :loading="waitingCam">Scan QR Code</q-btn>
  </div>
  <div v-if="errorMsg != null" class="flex flex-center q-pt-xs" style="color: red;">{{errorMsg}}</div>
  <div v-if="qrcodeOn" id="qrReader" class="flex flex-center q-pt-xs"></div>
  <div v-if="resultGet" class="flex flex-center q-ma-md">
    <q-img style="max-width: 100%;height: auto;" :src="srcImg"></q-img>
    <p>Do you want to authenticate this artefacts</p>
    <q-btn color="warning" @click="onConfirmClicked()">Confirm</q-btn>
  </div>
</q-page>
</transition>`,
  data() {
    return {
      customQr: null,
      errorMsg: null,
      waitingCam: false,
      qrcodeOn: false,
      resultGet: false,

      objId: "",
      srcImg: "",
    };
  },
  methods: {
    customQR() {
      this.errorMsg = null;
      if (this.customQr) {
        this.customQr
          .stop()
          .then((ignore) => {})
          .catch((err) => {});
        this.customQr = null;
        this.qrcodeOn = false;
        return;
      }
      this.qrcodeOn = true;
      this.resultGet = false;
      this.waitingCam = true;
      this.$nextTick(() => {
        this.customQr = new Html5Qrcode("qrReader");
        this.customQr
          .start(
            { facingMode: "environment" },
            { fps: 10, qrbox: 250, aspectRatio: 1.0 },
            this.onScanSuccess,
            this.onScanError
          )
          .catch((err) => {
            this.errorMsg = "No camera available";
            this.waitingCam = false;
          });
      });
    },
    onScanSuccess(msg) {
      this.errorMsg = null;
      if (
        msg.length == 36 &&
        msg[8] == "-" &&
        msg[13] == "-" &&
        msg[18] == "-" &&
        msg[23] == "-"
      ) {
        if (this.customQr) {
          this.customQr
            .stop()
            .then((ignore) => {})
            .catch((err) => {});
          this.customQr = null;
        }
        this.qrcodeOn = false;

        axios
          .post("/auth/api/chk", "id=" + msg)
          .then((r) => {
            if (r.data.msg == "OK") {
              this.resultGet = true;
              this.objId = msg;
              this.srcImg = "data:image/png;base64," + r.data.picb64;
            }
          })
          .catch((e) => {});
      } else {
        this.errorMsg = "Not a valid code";
      }
      this.waitingCam = false;
    },
    onScanError(err) {
      if (err.search("error = N;") != -1) {
        this.errorMsg = err;
      }
      this.waitingCam = false;
    },
    onConfirmClicked() {
      this.$router.push("/aiscan/" + this.objId);
    },
  },
};

const custom = {
  template: `<transition appear name="fade" mode="out-in">
<q-page>
  <h1 class="flex flex-center">Custom</h1>
</q-page>
</transition>`,
};

const notfound = {
  template: `<div class="fullscreen bg-primary text-white text-center flex flex-center">
  <div>
    <h1>404</h1><h2>Not Found</h2>
    <q-btn class="q-mt-xl" color="white" text-color="primary" unelevated to="/" label="Go Home" no-caps></q-btn>
  </div>
</div>
`,
};

const routes = [
  { path: "/", component: Home },
  { path: "/nfc", component: nfc },
  { path: "/:catchAll(.*)*", component: notfound },
];

function search(q) {
  console.log(q);
  return;
}

function addComponent() {
  //   app.component("InfoPlacer", {
  //     template: `
  // <div>{{value}}</div>
  //   `,
  //     props: {
  //       value: String,
  //     },
  //   });
}
