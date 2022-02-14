import axios from "axios";

// const endpoint = 'https://api.zinc.cse.ust.hk/v1';
const endpoint = 'https://api.oap.ust.dev/v1'
const httpClient = axios.create({
  baseURL: endpoint,
  headers: {
    'X-Hasura-Admin-Secret': process.env.HASURA_GRAPHQL_ADMIN_SECRET
  },
  method: 'post',
  maxContentLength: Infinity,
  maxBodyLength: Infinity
});

export default httpClient;
