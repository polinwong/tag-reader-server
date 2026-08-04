// 3D Artefact Exhibition server
// File: verify-aiscan.js, Ancient Artefacts Authentication - scan page script version vue
// Creater: Kevin Mak, October 2021
// (c)Marvel Digital Ltd. 2021

const uploadObj = {
  template: `
  <div class="q-ma-md uploadFile">
  <q-file
      v-model="curModel"
      clearable
      filled
      label="Capture or select a photo"
      accept=".jpg, image/*"
      @update:model-value="fileRead"
    ></q-file>
    <q-img
      class="q-mt-md"
      style="height: auto;"
      :src="imageData"
      :alt="imageName"
    ></q-img>
  </div>
`,
  props: {
    imageId: Number,
    imageName: String,
    imageData: String,
  },
  data() {
    return {
      curModel: null,
      curId: null,
    };
  },
  methods: {
    fileRead(v) {
      this.curModel = v;
      this.curId = this.imageId;
    },
  },
};

const aiScan = {
  components: {
    "upload-obj": uploadObj,
  },
  template: `<transition appear name="fade" mode="out-in">
  <q-page>
  <div v-if="showError"></div>
  <div class="flex flex-center">
    <h5>Please scan Artefacts</h5>
  </div>
  <div class="flex flex-center">
    <upload-obj
      v-for="item in srclist"
      :key="item.id"
      :imageId="item.id"
      :imageData="item.imageData"
      :imageName="item.imageName"
      :ref="setUpRefs"
    ></upload-obj>
  </div>
  <div v-if="loadListDone" class="flex flex-center">
    <q-btn :loading="waitingUpload" class="q-mb-md" color="primary" @click="sendData()">Submit photo</q-btn>
  </div>
</q-page>
</transition>
<q-dialog v-model="alertDialog">
  <q-card>
    <q-card-section>
      <div class="text-h6">Alert</div>
    </q-card-section>

    <q-card-section class="q-pt-none">
      {{alertMsg}}
    </q-card-section>

    <q-card-actions align="right">
      <q-btn flat label="OK" color="primary" v-close-popup />
    </q-card-actions>
  </q-card>
</q-dialog>
`,
  data() {
    return {
      objId: this.$route.params.id,
      srclist: [],
      upRefs: [],

      loadListDone: false,
      showError: false,
      msgError: "",

      alertDialog: false,
      alertMsg: "",
      waitingUpload: false,
    };
  },
  mounted() {
    if (!this.objId) {
      this.$router.replace("/");
    }
    axios
      .post("/auth/api/ref", "id=" + this.objId)
      .then((r) => {
        if (r.data.msg == "OK" && r.data.dataset.length > 0) {
          for (let i = 0; i < r.data.dataset.length; i++) {
            let src = "";
            switch (r.data.dataset[i].type) {
              case "png":
                src = "data:image/png;base64," + r.data.dataset[i].pic;
                break;
              case "jpg":
                src = "data:image/jpg;base64," + r.data.dataset[i].pic;
                break;
              default:
                this.msgError = "Reference image get error";
                return;
            }
            this.srclist.push({
              id: i,
              imageName: "Image" + i,
              imageData: src,
            });
          }
          this.loadListDone = true;
        }
      })
      .catch((e) => {});
  },
  beforeUpdate() {
    this.upRefs = [];
  },
  methods: {
    setUpRefs(el) {
      if (el && el.curModel) {
        this.upRefs.push({ id: el.curId, file: el.curModel });
      }
    },
    sendData() {
      this.waitingUpload = true;
      this.$forceUpdate();
      this.$nextTick(async () => {
        let numFiles = 0;
        for (let i in this.upRefs) {
          if (this.upRefs[i].file != null) {
            numFiles++;
          }
        }

        if (this.srclist.length != numFiles) {
          this.alertDialog = true;
          this.alertMsg = "Some file have not submit, please check it.";
          this.waitingUpload = false;
          return;
        }

        console.log("passed file check");
        try {
          let ret = await this.sendDataWork();
          if (ret.data.msg == "OK") {
            this.alertDialog = true;
            this.alertMsg =
              "Upload done, You Artefacts score " + ret.data.score;
          } else {
            this.alertMsg = ret.data.error;
          }
        } catch (e) {
          console.error(e);
          this.alertDialog = true;
          this.alertMsg = "Error: " + e.message;
        }

        this.waitingUpload = false;
      });
    },
    async sendDataWork() {
      // the demo of sync timeout
      // return new Promise((resolve, reject) => {
      //   setTimeout(() => {
      //     resolve('ok');
      //   }, 1000);
      // });
      let formData = new FormData();
      for (let i in this.upRefs) {
        formData.append("img" + this.upRefs[i].id, this.upRefs[i].file);
      }
      formData.append("num", this.upRefs.length);
      formData.append("id", this.objId);

      return axios.post("/auth/api/submit", formData, {
        headers: {
          "X-Requested-With": "XMLHttpRequest",
          "Content-Type": "multipart/form-data",
        },
      });
    },
  },
};
