# Setting up GCP

**Last Edited:** Astrid Gealer, 24th March 2026

1. Go to [Cloud Billing API](https://console.cloud.google.com/marketplace/product/google/cloudbilling.googleapis.com) and enable the API.
2. Click Manage, in the new page that opens up, click Credentials in the side bar.
3. Click "Manage Service Accounts", in the new page that opens up, click "Create Service Account".
4. Give it any ID you want. Click create, then click the email of the ID you just made.
5. In the top, click Permissions > Manage access > Add another role. Then under "Basic" click "Viewer" in the popup. After this, click Apply.
6. In the top, click Keys > Add Key. Then create a JSON key.

The following attributes from the JSON file should be your environment variables:

- `GCP_PROJECT_ID` > the value of `project_id` in the JSON
- `GCP_CLIENT_EMAIL` > the value of `client_email` in the JSON
- `GCP_PRIVATE_KEY` > the value of `private_key` in the JSON
