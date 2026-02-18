#!/bin/bash
# Provisions 1000 subscribers in free5GC v4.2.0 MongoDB.
#
# free5GC v4.x uses a different subscriber data model than v3.x:
#   - Auth: encPermanentKey/encOpcKey (flat strings) instead of permanentKey/opc (nested objects)
#   - SQN: sequenceNumber is a struct {sqnScheme, sqn, lastIndexes, indLength, difSign}
#   - SQN must start >= 32 (0x20) because UERANSIM's freshness check requires SEQ > 0
#     (SEQ = SQN >> indLength; with indLength=5, SQN=32 gives SEQ=1)
#
# Credentials (same as Open5GS baseline):
#   K:   465B5CE8B199B49FAA5F0A2EE238A6BC
#   OPC: E8ED289DEBA952E4283B54E88E6183CA
#   PLMN: 001/01, S-NSSAI: SST=1 SD=010203, DNN=internet
#
# Usage: ./provision-subscribers.sh [MONGODB_CONTAINER] [NUM_SUBSCRIBERS]
#   MONGODB_CONTAINER: default "free5gc-mongodb"
#   NUM_SUBSCRIBERS:   default 1000

set -euo pipefail

MONGO_CONTAINER="${1:-free5gc-mongodb}"
NUM_SUBS="${2:-1000}"

echo "Provisioning ${NUM_SUBS} subscribers in ${MONGO_CONTAINER}..."

docker exec "$MONGO_CONTAINER" mongosh --quiet --eval "
db = db.getSiblingDB('free5gc');

// Drop existing subscriber data
['subscriptionData.authenticationData.authenticationSubscription',
 'subscriptionData.provisionedData.amData',
 'subscriptionData.provisionedData.smData',
 'subscriptionData.provisionedData.smfSelectionSubscriptionData',
 'policyData.ues.amData',
 'policyData.ues.smData'].forEach(function(c) {
  db[c].drop();
});

var BATCH = 100;
var total = ${NUM_SUBS};

for (var batch = 0; batch < Math.ceil(total / BATCH); batch++) {
  var authDocs = [], amDocs = [], smDocs = [], smfSelDocs = [], policyAmDocs = [], policySmDocs = [];

  for (var i = batch * BATCH + 1; i <= Math.min((batch + 1) * BATCH, total); i++) {
    var imsi = 'imsi-001010' + ('000000000' + i).slice(-9);
    var msisdn = 'msisdn-09' + ('00000000' + i).slice(-8);

    authDocs.push({
      authenticationMethod: '5G_AKA',
      encPermanentKey: '465B5CE8B199B49FAA5F0A2EE238A6BC',
      encOpcKey: 'E8ED289DEBA952E4283B54E88E6183CA',
      authenticationManagementField: '8000',
      sequenceNumber: {
        sqnScheme: 'NON_TIME_BASED',
        sqn: '000000000020',
        lastIndexes: { 'ausf': NumberInt(0) },
        indLength: NumberInt(5),
        difSign: 'POSITIVE'
      },
      ueId: imsi,
      servingPlmnId: '00101'
    });

    amDocs.push({
      subscribedUeAmbr: { uplink: '1 Gbps', downlink: '2 Gbps' },
      nssai: { defaultSingleNssais: [{ sst: NumberInt(1), sd: '010203' }] },
      ueId: imsi, servingPlmnId: '00101', gpsis: [msisdn]
    });

    smDocs.push({
      singleNssai: { sst: NumberInt(1), sd: '010203' },
      dnnConfigurations: {
        internet: {
          pduSessionTypes: { defaultSessionType: 'IPV4', allowedSessionTypes: ['IPV4'] },
          sscModes: { allowedSscModes: ['SSC_MODE_1','SSC_MODE_2','SSC_MODE_3'], defaultSscMode: 'SSC_MODE_1' },
          '5gQosProfile': { '5qi': NumberInt(9), arp: { priorityLevel: NumberInt(8), preemptCap: '', preemptVuln: '' } },
          sessionAmbr: { downlink: '100 Mbps', uplink: '200 Mbps' }
        }
      },
      ueId: imsi, servingPlmnId: '00101'
    });

    smfSelDocs.push({
      subscribedSnssaiInfos: { '01010203': { dnnInfos: [{ dnn: 'internet' }] } },
      ueId: imsi, servingPlmnId: '00101'
    });

    policyAmDocs.push({ ueId: imsi, subscCats: ['free5gc'] });

    policySmDocs.push({
      smPolicySnssaiData: {
        '01010203': {
          snssai: { sst: NumberInt(1), sd: '010203' },
          smPolicyDnnData: { internet: { dnn: 'internet' } }
        }
      },
      ueId: imsi
    });
  }

  db['subscriptionData.authenticationData.authenticationSubscription'].insertMany(authDocs);
  db['subscriptionData.provisionedData.amData'].insertMany(amDocs);
  db['subscriptionData.provisionedData.smData'].insertMany(smDocs);
  db['subscriptionData.provisionedData.smfSelectionSubscriptionData'].insertMany(smfSelDocs);
  db['policyData.ues.amData'].insertMany(policyAmDocs);
  db['policyData.ues.smData'].insertMany(policySmDocs);

  print('  Batch ' + (batch + 1) + ': ' + authDocs.length + ' subscribers');
}

print('Done. Total: ' + db['subscriptionData.authenticationData.authenticationSubscription'].countDocuments());
"

echo "Provisioning complete."
