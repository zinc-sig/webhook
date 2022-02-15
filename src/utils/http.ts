import axios from "axios";

const endpoint = `http://${process.env.HASURA_ADDR}/v1`
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
